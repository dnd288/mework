package session

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"mework/libs/server/auth"
	"mework/libs/server/bus"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// Dispatcher publishes an open-session dispatch to a named runner. It is
// satisfied by *catalog.AgentHandlers; injected so the session handlers can be
// tested without the full catalog.
type Dispatcher interface {
	DispatchSessionToRunner(ctx context.Context, agentName, runnerID, sessionID, owner, tenant, workspace string, g *grant.Grant) error
}

// Handlers provides HTTP handlers for the session API.
type Handlers struct {
	manager  *Manager
	dispatch Dispatcher
	broker   bus.Broker
	sse      *bus.SSEHandler
}

// NewHandlers creates a new Handlers backed by the given Manager, dispatcher, and
// message broker. Session-create dispatches an open-session message to the
// request's runner via the dispatcher (c0031); the broker publishes chat turns to
// the session input topic and relays outgoing events from the session control
// topic (c0032).
func NewHandlers(manager *Manager, dispatch Dispatcher, broker bus.Broker) *Handlers {
	return &Handlers{
		manager:  manager,
		dispatch: dispatch,
		broker:   broker,
		sse:      bus.NewSSEHandler(broker),
	}
}

// --- request / response types ------------------------------------------------

type createSessionRequest struct {
	AgentName string `json:"agent_name"`
	Version   string `json:"version,omitempty"`
	Runner    string `json:"runner"`
	// Workspace, when set, is an absolute local directory the runner binds the
	// session's sandbox to (resolving its definition from the dir's mework.yml).
	Workspace string `json:"workspace,omitempty"`
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
			r.Context(), req.AgentName, req.Runner, string(info.ID), string(owner), string(tenant), req.Workspace, g,
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

// SendMessage handles POST /api/v1/sessions/{id}/messages. It decodes a
// ChatMessage, verifies the caller owns the session, and publishes the turn to
// session.<id>.input for the running sandbox to consume. The server records
// nothing — it is a thin relay — and returns 202 Accepted on success.
func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	var msg ChatMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if !h.ownsSession(r, id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		http.Error(w, "encode message", http.StatusInternalServerError)
		return
	}

	topic := bus.FormatTopic(bus.TopicSessionInput, string(id))
	if err := h.broker.Publish(r.Context(), topic, bus.Message{Payload: payload}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// StreamSession handles GET /api/v1/sessions/{id}/stream. It verifies ownership
// then relays the session control topic (session.<id>.control) to the caller as
// Server-Sent Events, reusing bus.SSEHandler for heartbeat, resume, and bounded
// backpressure.
func (h *Handlers) StreamSession(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	if !h.ownsSession(r, id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Target the session control topic by injecting it into the request the
	// SSE handler reads its "topics" query parameter from.
	controlTopic := string(bus.FormatTopic(bus.TopicSessionControl, string(id)))
	q := r.URL.Query()
	q.Set("topics", controlTopic)
	r2 := r.Clone(r.Context())
	r2.URL.RawQuery = q.Encode()

	h.sse.Subscribe(w, r2)
}

// ReceiveEvents handles POST /api/v1/runners/sessions/{id}/events. It is the
// runtime-authed ingress for the daemon, which has no in-process server broker:
// it republishes a raw ChatEvent JSON body to session.<id>.control so the hub's
// stream relays it. Returns 202 Accepted on success.
func (h *Handlers) ReceiveEvents(w http.ResponseWriter, r *http.Request) {
	id := core.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	var ev ChatEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payload, err := json.Marshal(ev)
	if err != nil {
		http.Error(w, "encode event", http.StatusInternalServerError)
		return
	}

	topic := bus.FormatTopic(bus.TopicSessionControl, string(id))
	if err := h.broker.Publish(r.Context(), topic, bus.Message{Payload: payload}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// ownsSession reports whether the authenticated caller owns the given session.
// A session that cannot be resolved is treated as not owned.
func (h *Handlers) ownsSession(r *http.Request, id core.SessionID) bool {
	info, err := h.manager.Get(r.Context(), id)
	if err != nil {
		return false
	}
	caller, ok := auth.GetAccountID(r.Context())
	if !ok || caller == "" {
		return false
	}
	return info.Owner == core.AccountID(caller)
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
