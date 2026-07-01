package unitqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mework/libs/server/auth"
	"mework/libs/server/bus"
	"mework/libs/shared/policy"
)

// PolicyLoader resolves the message policy for a named agent. When nil,
// no policy enforcement is applied (backward-compatible default).
type PolicyLoader interface {
	LoadPolicy(ctx context.Context, agentName string) (*policy.Policy, error)
}

// Handlers provides HTTP handlers for the unit queue API — register, deregister,
// list, lookup, and send-message-by-name.
type Handlers struct {
	registry    Registry
	broker      bus.Broker
	policy      PolicyLoader
	rateLimiter *policy.RateLimiter
}

// NewHandlers creates handlers backed by the given Registry and Broker.
func NewHandlers(registry Registry, broker bus.Broker) *Handlers {
	return &Handlers{
		registry:    registry,
		broker:      broker,
		rateLimiter: policy.NewRateLimiter(),
	}
}

// SetPolicyLoader sets the policy loader for message-level policy enforcement.
// When set, every SendMessage call checks the agent's policy before routing.
func (h *Handlers) SetPolicyLoader(pl PolicyLoader) {
	h.policy = pl
}

// --- request / response helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, msg string, status int) {
	http.Error(w, msg, status)
}

// --- handlers ---

type registerRequest struct {
	SessionID string `json:"session_id"`
	RunnerID  string `json:"runner_id"`
}

// RegisterAgent handles POST /api/v1/unitqueues/{name}/register.
// Binds name → sessionID so messages can be routed by name.
func (h *Handlers) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpError(w, "missing name", http.StatusBadRequest)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.RunnerID == "" {
		httpError(w, "session_id and runner_id are required", http.StatusBadRequest)
		return
	}

	if err := h.registry.Register(r.Context(), name, req.SessionID, req.RunnerID); err != nil {
		httpError(w, err.Error(), http.StatusConflict)
		return
	}

	reg, _ := h.registry.Lookup(r.Context(), name)
	writeJSON(w, http.StatusCreated, reg)
}

// DeregisterAgent handles POST /api/v1/unitqueues/{name}/deregister.
// Removes the name binding, making the agent offline.
func (h *Handlers) DeregisterAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpError(w, "missing name", http.StatusBadRequest)
		return
	}

	if err := h.registry.Unregister(r.Context(), name); err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListAgents handles GET /api/v1/unitqueues.
// Returns all registered (online) unit queues.
func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	list, err := h.registry.List(r.Context())
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []Registration{}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetAgent handles GET /api/v1/unitqueues/{name}.
// Returns the registration status for a single unit queue.
func (h *Handlers) GetAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpError(w, "missing name", http.StatusBadRequest)
		return
	}

	reg, err := h.registry.Lookup(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, reg)
}

// enforcePolicy checks the agent's policy rules for the given message.
// Returns deny result when the message violates policy.
func (h *Handlers) enforcePolicy(r *http.Request, agentName, content string) (*policy.Result, error) {
	if h.policy == nil {
		return &policy.Result{Allowed: true}, nil
	}

	// Build attributes from the request context.
	attrs := policy.Attributes{
		"content":         content,
		"content_length":  fmt.Sprint(len(content)),
		"time":            time.Now().UTC().Format(time.RFC3339),
		"channel":         "hub",
	}
	if acct, ok := auth.GetAccountID(r.Context()); ok {
		attrs["sender"] = acct
		attrs["authenticated"] = "true"
	} else {
		attrs["sender"] = "anonymous"
		attrs["authenticated"] = "false"
	}

	p, err := h.policy.LoadPolicy(r.Context(), agentName)
	if err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}
	if p == nil {
		return &policy.Result{Allowed: true}, nil
	}

	result, err := p.Enforce(attrs)
	if err != nil {
		return nil, err
	}

	// Handle rate_limit action: check the rate limiter.
	if result.Reason != "" {
		if count, ok := policy.ParseLimit(result.Reason); ok {
			if !h.rateLimiter.Allow(attrs["sender"], count) {
				return &policy.Result{Allowed: false, Reason: "rate limit exceeded"}, nil
			}
		}
	}

	return result, nil
}

// chatMessage is the inbound turn from a human operator, matching
// session.ChatMessage but kept self-contained to avoid a direct dependency on
// the session package.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SendMessage handles POST /api/v1/unitqueues/{name}/messages.
// Resolves name → session, then publishes the chat turn to session.<id>.input
// (the same topic the session send-message handler publishes to). Returns 202
// with the target session_id.
func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpError(w, "missing name", http.StatusBadRequest)
		return
	}

	// Resolve name → session.
	reg, err := h.registry.Lookup(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	var msg chatMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg.Content == "" {
		httpError(w, "message content is required", http.StatusBadRequest)
		return
	}

	// ---- POLICY ENFORCEMENT ----
	result, err := h.enforcePolicy(r, name, msg.Content)
	if err != nil {
		httpError(w, "policy error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !result.Allowed {
		httpError(w, result.Reason, http.StatusForbidden)
		return
	}
	// ---- END POLICY ENFORCEMENT ----

	// Wrap in the same inputMessage envelope the daemon expects.
	// See libs/client/runner/session_dispatch.go:inputMessage.
	wrapper := struct {
		Message chatMessage `json:"message"`
	}{Message: msg}
	payload, err := json.Marshal(wrapper)
	if err != nil {
		httpError(w, "encode message", http.StatusInternalServerError)
		return
	}

	topic := bus.FormatTopic(bus.TopicSessionInput, reg.SessionID)
	if err := h.broker.Publish(r.Context(), topic, bus.Message{Payload: payload}); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":     "accepted",
		"name":       name,
		"session_id": reg.SessionID,
	})
}

// Ensure the handler satisfies middleware-friendly interface.
var _ = (*Handlers).RegisterAgent
var _ = (*Handlers).DeregisterAgent
var _ = (*Handlers).ListAgents
var _ = (*Handlers).GetAgent
var _ = (*Handlers).SendMessage
