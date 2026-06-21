package session

import (
	"encoding/json"
	"net/http"

	"mework/libs/server/auth"
	"mework/libs/shared/core"
)

// Handlers provides HTTP handlers for the session API.
type Handlers struct {
	manager *Manager
}

// NewHandlers creates a new Handlers backed by the given Manager.
func NewHandlers(manager *Manager) *Handlers {
	return &Handlers{manager: manager}
}

// --- request / response types ------------------------------------------------

type createSessionRequest struct {
	AgentName string `json:"agent_name"`
	Version   string `json:"version,omitempty"`
	Runner    string `json:"runner"`
}

// CreateSession handles POST /api/v1/sessions.
func (h *Handlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.AgentName == "" || req.Runner == "" {
		http.Error(w, "agent_name and runner are required", http.StatusBadRequest)
		return
	}

	owner := core.AccountID("")
	tenant := core.TenantID("")
	if acct, ok := auth.GetAccountID(r.Context()); ok {
		owner = core.AccountID(acct)
	}
	if tn, ok := auth.GetTenantID(r.Context()); ok {
		tenant = core.TenantID(tn)
	}

	info, err := h.manager.Create(r.Context(), req.AgentName, req.Version, req.Runner, owner, tenant)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, info)
}

// GetSession handles GET /api/v1/sessions/{id}.
func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	info, err := h.manager.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// ListSessions handles GET /api/v1/sessions.
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	tenant := core.TenantID("")
	if tn, ok := auth.GetTenantID(r.Context()); ok {
		tenant = core.TenantID(tn)
	}

	list, err := h.manager.List(r.Context(), tenant)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

// AttachSession handles POST /api/v1/sessions/{id}/attach.
func (h *Handlers) AttachSession(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	sess, err := h.manager.Attach(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Return session metadata; the caller uses the /api/v1/jobs/subscribe
	// endpoint with the session control topic for the live stream.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":      sess.ID(),
		"control": "session." + string(sess.ID()) + ".control",
	})
}

// CloseSession handles DELETE /api/v1/sessions/{id}.
func (h *Handlers) CloseSession(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	if err := h.manager.Close(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
