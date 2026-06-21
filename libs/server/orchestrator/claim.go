package orchestrator

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/middleware"
)

type Job struct {
	ID                  string  `json:"id"`
	AccountID           string  `json:"account_id"`
	RuntimeID           string  `json:"runtime_id"`
	ExternalTaskID      string  `json:"external_task_id"`
	ExternalEventID     string  `json:"external_event_id"`
	ProviderCode        string  `json:"provider_code"`
	ExternalActorID     *string `json:"external_actor_id,omitempty"`
	TaskTitle           string  `json:"task_title"`
	TaskDescription     string  `json:"task_description"`
	ProfileBodySnapshot *string `json:"profile_body_snapshot,omitempty"`
	Workflow            *string `json:"workflow,omitempty"`
	Instructions        string  `json:"instructions"`
	Status              string  `json:"status"`
}

type ClaimHandlers struct {
	pool *pgxpool.Pool
}

func NewClaimHandlers(pool *pgxpool.Pool) *ClaimHandlers {
	return &ClaimHandlers{pool: pool}
}

// ClaimJob handles the POST /api/v1/jobs/claim request.
func (h *ClaimHandlers) ClaimJob(w http.ResponseWriter, r *http.Request) {
	runtimeID, ok := middleware.GetRuntimeID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing runtime ID", http.StatusUnauthorized)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		logError("Claim job transaction start failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// 1. Concurrency limit: pg_advisory_xact_lock to serialize claims per runtime (H7)
	_, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext($1))", runtimeID)
	if err != nil {
		logError("Advisory lock failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 2. One-job-per-runtime check: check if there's already an active (claimed/running) job
	var activeCount int
	err = tx.QueryRow(r.Context(), `
		SELECT count(*) FROM jobs
		WHERE runtime_id = $1 AND status IN ('claimed', 'running')
	`, runtimeID).Scan(&activeCount)
	if err != nil {
		logError("Query active count failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if activeCount > 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 3. Claim oldest queued job
	var job Job
	err = tx.QueryRow(r.Context(), `
		UPDATE jobs
		SET status = 'claimed', claim_lease_until = NOW() + INTERVAL '30 seconds', attempts = attempts + 1
		WHERE id = (
			SELECT id FROM jobs
			WHERE runtime_id = $1 AND status = 'queued'
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, account_id, runtime_id, external_task_id, external_event_id, provider_code, external_actor_id, task_title, task_description, profile_body_snapshot, workflow, instructions, status
	`, runtimeID).Scan(
		&job.ID, &job.AccountID, &job.RuntimeID, &job.ExternalTaskID, &job.ExternalEventID, &job.ProviderCode,
		&job.ExternalActorID, &job.TaskTitle, &job.TaskDescription, &job.ProfileBodySnapshot, &job.Workflow, &job.Instructions, &job.Status,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		logError("Claim oldest job query failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		logError("Transaction commit failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

func logError(msg string, err error) {
	log.Printf("ERROR: %s: %v", msg, err)
}
