package session

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"mework/libs/server/auth"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// Dispatcher publishes an open-session dispatch to a named runner. It is
// satisfied by *catalog.AgentHandlers; injected so the session handlers can be
// tested without the full catalog.
type Dispatcher interface {
	DispatchSessionToRunner(ctx context.Context, agentName, runnerID, sessionID, owner, tenant string, g *grant.Grant) error
}

// Handlers provides HTTP handlers for the session API.
type Handlers struct {
	manager  *Manager
	dispatch Dispatcher
}

// NewHandlers creates a new Handlers backed by the given Manager and dispatcher.
// Session-create dispatches an open-session message to the request's runner via
// the dispatcher.
func NewHandlers(manager *Manager, dispatch Dispatcher) *Handlers {
	return &Handlers{manager: manager, dispatch: dispatch}
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

	// Dispatch an open-session message to the named runner with a pull+spawn
	// grant. The non-empty session id is the signal the daemon uses to open a
	// long-lived sandbox; owner/tenant let the runner authorize turns.
	if h.dispatch != nil {
		g, gerr := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, nil)
		if gerr != nil {
			http.Error(w, gerr.Error(), http.StatusInternalServerError)
			return
		}
		if derr := h.dispatch.DispatchSessionToRunner(
			r.Context(), req.AgentName, req.Runner, string(info.ID), string(owner), string(tenant), g,
		); derr != nil {
			// The session is already created; dispatch is best-effort (the
			// runner may not be subscribed yet). Log but don't fail the create,
			// mirroring the channel provisioner.
			log.Printf("dispatch session %s to runner %s: %v", info.ID, req.Runner, derr)
		}
	}

	writeJSON(w, http.StatusCreated, info)
}

// resultRequest is the terminal result body the daemon POSTs for a session,
// matching libs/client/runner/dispatch.go's reportResult.
type resultRequest struct {
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ResultSession handles POST /api/v1/runners/sessions/{id}/result. It is a thin
// status sink (runtime-authed): it records the terminal result and returns 204
// so the daemon's POST does not 404. It MAY publish a terminal ChatEvent on the
// session control topic.
func (h *Handlers) ResultSession(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	var req resultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("session %s result: status=%s summary=%q error=%q", id, req.Status, req.Summary, req.Error)
	w.WriteHeader(http.StatusNoContent)
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
