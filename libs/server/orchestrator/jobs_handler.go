package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/middleware"
)

// jobResponse represents a single job in the list endpoint JSON response.
type jobResponse struct {
	ID            string  `json:"id"`
	ProviderCode  string  `json:"provider_code"`
	Status        string  `json:"status"`
	ChannelID     string  `json:"channel_id"`
	Instructions  string  `json:"instructions"`
	ResultSummary *string `json:"result_summary,omitempty"`
}

// JobsHandlers holds dependencies for the jobs list HTTP endpoint.
type JobsHandlers struct {
	pool *pgxpool.Pool
}

// NewJobsHandlers creates a new JobsHandlers.
func NewJobsHandlers(pool *pgxpool.Pool) *JobsHandlers {
	return &JobsHandlers{pool: pool}
}

// ListJobs handles GET /api/v1/jobs with optional query params: provider, status, since.
func (h *JobsHandlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	status := r.URL.Query().Get("status")
	since := r.URL.Query().Get("since")

	// Validate status if provided.
	if status != "" {
		validStatuses := map[string]bool{
			"queued":  true,
			"claimed": true,
			"running": true,
			"done":    true,
			"failed":  true,
		}
		if !validStatuses[status] {
			http.Error(w, "Bad Request: invalid status", http.StatusBadRequest)
			return
		}
	}

	// Build query with dynamic WHERE clauses.
	query := `SELECT id, provider_code, status, external_task_id AS channel_id, instructions, result_summary FROM jobs WHERE 1=1`
	args := []any{}
	argNum := 0

	// Scope query to the authenticated runtime's account.
	accountID, ok := middleware.GetAccountID(r.Context())
	if ok {
		argNum++
		query += fmt.Sprintf(" AND account_id = $%d", argNum)
		args = append(args, accountID)
	}

	if provider != "" {
		argNum++
		query += fmt.Sprintf(" AND provider_code = $%d", argNum)
		args = append(args, provider)
	}
	if status != "" {
		argNum++
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, status)
	}
	if since != "" {
		argNum++
		query += fmt.Sprintf(" AND created_at > (SELECT created_at FROM jobs WHERE id = $%d)", argNum)
		args = append(args, since)
	}
	query += " ORDER BY created_at ASC LIMIT 100"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		logError("List jobs query failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	jobs := make([]jobResponse, 0)
	for rows.Next() {
		var job jobResponse
		if err := rows.Scan(&job.ID, &job.ProviderCode, &job.Status, &job.ChannelID, &job.Instructions, &job.ResultSummary); err != nil {
			logError("Scan job row failed", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		logError("Iterate job rows failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}
