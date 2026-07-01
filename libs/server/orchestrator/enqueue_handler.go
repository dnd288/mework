package orchestrator

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/middleware"
)

// enqueueRequest is the JSON body for the POST /api/v1/jobs/enqueue endpoint.
type enqueueRequest struct {
	ProviderCode string `json:"provider_code"`
	ChannelID    string `json:"channel_id"`
	SenderID     string `json:"sender_id"`
	Text         string `json:"text"`
	MessageID    string `json:"message_id"`
}

// enqueueResponse is the JSON response body for the job enqueue endpoint.
type enqueueResponse struct {
	JobID string `json:"job_id"`
}

// EnqueueHandlers holds dependencies for the enqueue HTTP endpoint.
type EnqueueHandlers struct {
	pool *pgxpool.Pool
}

// NewEnqueueHandlers creates a new EnqueueHandlers.
func NewEnqueueHandlers(pool *pgxpool.Pool) *EnqueueHandlers {
	return &EnqueueHandlers{pool: pool}
}

// EnqueueJob handles POST /api/v1/jobs/enqueue.
func (h *EnqueueHandlers) EnqueueJob(w http.ResponseWriter, r *http.Request) {
	runtimeID, ok := middleware.GetRuntimeID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing runtime ID", http.StatusUnauthorized)
		return
	}

	accountID, ok := middleware.GetAccountID(r.Context())
	if !ok {
		// Fallback: look up account ID from the runtime row.
		err := h.pool.QueryRow(r.Context(),
			"SELECT account_id FROM runtimes WHERE id = $1", runtimeID,
		).Scan(&accountID)
		if err != nil {
			logError("Lookup runtime account ID failed", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	var req enqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	switch {
	case req.ProviderCode == "":
		http.Error(w, "Bad Request: provider_code is required", http.StatusBadRequest)
		return
	case req.ChannelID == "":
		http.Error(w, "Bad Request: channel_id is required", http.StatusBadRequest)
		return
	case req.Text == "":
		http.Error(w, "Bad Request: text is required", http.StatusBadRequest)
		return
	case req.MessageID == "":
		http.Error(w, "Bad Request: message_id is required", http.StatusBadRequest)
		return
	}

	if len(req.Text) > 65536 {
		http.Error(w, "Request Entity Too Large: text exceeds 64KB limit", http.StatusRequestEntityTooLarge)
		return
	}

	params := EnqueueParams{
		AccountID:       accountID,
		RuntimeID:       runtimeID,
		ExternalTaskID:  req.ChannelID,
		ExternalEventID: req.MessageID,
		ProviderCode:    req.ProviderCode,
		ExternalActorID: req.SenderID,
		Instructions:    req.Text,
	}

	jobID, err := Enqueue(r.Context(), h.pool, params)
	if err != nil {
		logError("Enqueue job failed", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if jobID == "" {
		// Duplicate: look up the existing job ID by unique constraint.
		err = h.pool.QueryRow(r.Context(), `
			SELECT id FROM jobs
			WHERE provider_code = $1 AND external_event_id = $2
		`, req.ProviderCode, req.MessageID).Scan(&jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				logError("Dedup lookup found no existing job", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			logError("Dedup lookup query failed", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(enqueueResponse{JobID: jobID})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(enqueueResponse{JobID: jobID})
}
