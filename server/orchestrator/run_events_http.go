package orchestrator

import (
	"encoding/json"
	"net/http"

	"mework/shared/transport"
)

// RunEventsHandlers provides HTTP handlers for run event operations:
// status queries and cancellation.
type RunEventsHandlers struct {
	svc transport.RunEvents
}

// NewRunEventsHandlers returns a new RunEventsHandlers backed by the given service.
func NewRunEventsHandlers(svc transport.RunEvents) *RunEventsHandlers {
	return &RunEventsHandlers{svc: svc}
}

// Status handles GET /api/v1/runs/{runID}/status.
func (h *RunEventsHandlers) Status(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	status, err := h.svc.Status(r.Context(), runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"run_id": runID, "status": string(status)})
}

// Cancel handles POST /api/v1/runs/{runID}/cancel.
func (h *RunEventsHandlers) Cancel(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Force bool `json:"force,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.svc.Cancel(r.Context(), runID, req.Force); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"canceled"}`))
}

// Emit handles POST /api/v1/runs/{runID}/events for runner/agent upstream emission.
func (h *RunEventsHandlers) Emit(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	var ev transport.RunEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "invalid event body", http.StatusBadRequest)
		return
	}

	if err := h.svc.Emit(r.Context(), runID, ev); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}
