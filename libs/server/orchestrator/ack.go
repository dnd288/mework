package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/middleware"
	"mework/libs/server/writeback"
)

type AckRequest struct {
	Status        string  `json:"status"`
	ResultSummary *string `json:"result_summary,omitempty"`
	LastError     *string `json:"last_error,omitempty"`
}

type AckHandlers struct {
	pool         *pgxpool.Pool
	secretKey    string
	melloBaseURL string
}

func NewAckHandlers(pool *pgxpool.Pool, secretKey, melloBaseURL string) *AckHandlers {
	return &AckHandlers{
		pool:         pool,
		secretKey:    secretKey,
		melloBaseURL: melloBaseURL,
	}
}

// AckJob handles the POST /api/v1/jobs/:id/ack request.
func (h *AckHandlers) AckJob(w http.ResponseWriter, r *http.Request) {
	runtimeID, ok := middleware.GetRuntimeID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing runtime ID", http.StatusUnauthorized)
		return
	}

	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		http.Error(w, "Bad Request: missing job ID", http.StatusBadRequest)
		return
	}

	var req AckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	if req.Status != "running" && req.Status != "done" && req.Status != "failed" {
		http.Error(w, "Bad Request: invalid status parameter", http.StatusBadRequest)
		return
	}

	// 1. Ownership check: check if the job belongs to this runtime
	var jobRuntimeID string
	err := h.pool.QueryRow(r.Context(), "SELECT runtime_id FROM jobs WHERE id = $1", jobID).Scan(&jobRuntimeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		logError("Query job ownership failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if jobRuntimeID != runtimeID {
		http.Error(w, "Forbidden: runtime does not own this job", http.StatusForbidden)
		return
	}

	// 2. Transition state
	err = TransitionJobState(r.Context(), h.pool, jobID, req.Status)
	if err != nil {
		if errors.Is(err, ErrInvalidTransition) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		logError("Transition job state failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 3. Update result_summary/last_error for terminal state and trigger write-back
	if req.Status == "done" || req.Status == "failed" {
		var summary, lastErr string
		if req.ResultSummary != nil {
			summary = *req.ResultSummary
		}
		if req.LastError != nil {
			lastErr = *req.LastError
		}

		_, err = h.pool.Exec(r.Context(), `
			UPDATE jobs
			SET result_summary = $1, last_error = $2, writeback_status = 'pending'
			WHERE id = $3
		`, summary, lastErr, jobID)
		if err != nil {
			logError("Failed to update terminal job results", err)
		}

		// Asynchronously trigger REST write-back
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()

			wbErr := writeback.ExecuteWriteBack(bgCtx, h.pool, h.secretKey, h.melloBaseURL, jobID)
			if wbErr != nil {
				log.Printf("Asynchronous write-back for job %s failed: %v", jobID, wbErr)
				_, dbErr := h.pool.Exec(context.Background(), `
					UPDATE jobs
					SET writeback_last_error = $1
					WHERE id = $2
				`, wbErr.Error(), jobID)
				if dbErr != nil {
					log.Printf("Failed to record writeback error for job %s: %v", jobID, dbErr)
				}
			} else {
				_, dbErr := h.pool.Exec(context.Background(), `
					UPDATE jobs
					SET writeback_status = 'success'
					WHERE id = $1
				`, jobID)
				if dbErr != nil {
					log.Printf("Failed to record writeback success for job %s: %v", jobID, dbErr)
				}
			}
		}()
	}

	w.WriteHeader(http.StatusNoContent)
}

// Heartbeat handles the POST /api/v1/jobs/:id/heartbeat request.
func (h *AckHandlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	runtimeID, ok := middleware.GetRuntimeID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing runtime ID", http.StatusUnauthorized)
		return
	}

	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		http.Error(w, "Bad Request: missing job ID", http.StatusBadRequest)
		return
	}

	// 1. Ownership check
	var jobRuntimeID string
	err := h.pool.QueryRow(r.Context(), "SELECT runtime_id FROM jobs WHERE id = $1", jobID).Scan(&jobRuntimeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		logError("Query job ownership for heartbeat failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if jobRuntimeID != runtimeID {
		http.Error(w, "Forbidden: runtime does not own this job", http.StatusForbidden)
		return
	}

	// 2. Extend claim lease until NOW() + 90 seconds
	_, err = h.pool.Exec(r.Context(), `
		UPDATE jobs
		SET claim_lease_until = NOW() + INTERVAL '90 seconds'
		WHERE id = $1
	`, jobID)

	if err != nil {
		logError("Failed to update heartbeat lease", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
