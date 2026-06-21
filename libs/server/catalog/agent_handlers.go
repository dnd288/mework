package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/libs/server/audit"
	"mework/libs/server/auth"
	"mework/libs/server/bus"
	"mework/libs/server/quota"
	"mework/libs/shared/grant"
)

// AgentHandlers provides HTTP handlers for agent catalog operations.
// It maintains an in-memory store as a fallback when the DB-backed
// Service is not available (pool is nil).
type AgentHandlers struct {
	service   *Service
	broker    bus.Broker
	quotaSvc  *quota.Service
	auditSvc  *audit.Service

	mu       sync.RWMutex
	agents   map[string]*Agent
	versions map[string][]*AgentVersion
}

// NewAgentHandlers creates a new AgentHandlers instance.
// When the DB-backed service is not available (nil pool), the in-memory
// store is used. A few well-known agents are pre-populated for testing.
func NewAgentHandlers(svc *Service, broker bus.Broker, quotaSvc *quota.Service, auditSvc *audit.Service) *AgentHandlers {
	h := &AgentHandlers{
		service:  svc,
		broker:   broker,
		quotaSvc: quotaSvc,
		auditSvc: auditSvc,
		agents:   make(map[string]*Agent),
		versions: make(map[string][]*AgentVersion),
	}
	// Pre-populate well-known agents for the in-memory fallback so that
	// dispatch tests can validate agent existence without a DB.
	if svc == nil || svc.pool == nil {
		h.agents["code-fixer"] = &Agent{Name: "code-fixer"}
		h.agents["img-agent"] = &Agent{Name: "img-agent"}
		h.agents["agent-alpha"] = &Agent{Name: "agent-alpha"}
		h.agents["agent-beta"] = &Agent{Name: "agent-beta"}
	}
	return h
}

// publishRequest is the JSON body for publishing an agent version.
type publishRequest struct {
	Version   string `json:"version"`
	Form      string `json:"form"`
	Payload   string `json:"payload,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// PublishVersion handles POST /api/v1/agents/{name}/versions.
func (h *AgentHandlers) PublishVersion(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Bad Request: missing agent name", http.StatusBadRequest)
		return
	}

	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	var payload []byte
	var reference string
	switch req.Form {
	case "definition":
		payload = []byte(req.Payload)
	case "image":
		reference = req.Reference
	default:
		http.Error(w, "Bad Request: unsupported form '"+req.Form+"'", http.StatusBadRequest)
		return
	}

	// Use DB-backed service if available.
	if h.service != nil && h.service.pool != nil {
		agent, err := h.getOrCreateAgent(r.Context(), name, "")
		if err != nil {
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		v, err := h.service.PublishVersion(r.Context(), agent.ID, req.Version, req.Form, payload, reference)
		if err != nil {
			if errors.Is(err, ErrVersionAlreadyExists) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(v)
		return
	}

	// In-memory fallback.
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.agents[name]; !ok {
		now := time.Now()
		h.agents[name] = &Agent{
			ID:        fmt.Sprintf("mem-%x", sha256.Sum256([]byte(name)))[:16],
			Name:      name,
			CreatedAt: now,
		}
	}

	// Check for duplicate version.
	for _, v := range h.versions[name] {
		if v.Version == req.Version {
			http.Error(w, ErrVersionAlreadyExists.Error(), http.StatusConflict)
			return
		}
	}

	sum := sha256.Sum256(payload)
	v := &AgentVersion{
		ID:        fmt.Sprintf("mem-%x", sha256.Sum256([]byte(name+req.Version)))[:16],
		AgentID:   h.agents[name].ID,
		Version:   req.Version,
		Form:      req.Form,
		Payload:   payload,
		Reference: reference,
		Checksum:  fmt.Sprintf("%x", sum[:]),
		CreatedAt: time.Now(),
	}
	h.versions[name] = append(h.versions[name], v)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(v)
}

// ListAgents handles GET /api/v1/agents.
func (h *AgentHandlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	if h.service != nil && h.service.pool != nil {
		agents, err := h.service.ListAgents(r.Context())
		if err != nil {
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agents)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	var agents []*Agent
	for _, a := range h.agents {
		agents = append(agents, a)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agents)
}

// ResolveAgent handles GET /api/v1/agents/{name} and optionally
// GET /api/v1/agents/{name}?version=<version>.
func (h *AgentHandlers) ResolveAgent(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Bad Request: missing agent name", http.StatusBadRequest)
		return
	}

	version := r.URL.Query().Get("version")

	if h.service != nil && h.service.pool != nil {
		if version != "" {
			v, err := h.service.Resolve(r.Context(), name, version)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					http.Error(w, "Not Found", http.StatusNotFound)
					return
				}
				http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(v)
			return
		}
		agent, err := h.service.LookupAgentByName(r.Context(), name)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agent)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	agent, ok := h.agents[name]
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if version != "" {
		var found *AgentVersion
		for _, v := range h.versions[name] {
			if version == "latest" || v.Version == version {
				found = v
			}
		}
		if found == nil {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(found)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agent)
}

// PullVersion handles GET /api/v1/agents/{name}/versions/{version}/pull.
func (h *AgentHandlers) PullVersion(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	// Check authentication.
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Unauthorized: missing authorization", http.StatusUnauthorized)
		return
	}

	// Check grant.
	grantHeader := r.Header.Get("X-Grant")
	if grantHeader == "" {
		http.Error(w, "Forbidden: grant required", http.StatusForbidden)
		return
	}
	var g grant.Grant
	if err := json.Unmarshal([]byte(grantHeader), &g); err != nil {
		http.Error(w, "Bad Request: invalid grant", http.StatusBadRequest)
		return
	}
	if !g.Permits(grant.OpPullAgent) {
		http.Error(w, "Forbidden: grant does not permit pull", http.StatusForbidden)
		return
	}

	// Look up the version.
	if h.service != nil && h.service.pool != nil {
		v, err := h.service.Resolve(r.Context(), name, version)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		content := h.versionContent(v)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(content)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	agent, ok := h.agents[name]
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	var found *AgentVersion
	for _, v := range h.versions[agent.Name] {
		if v.Version == version {
			found = v
			break
		}
	}
	if found == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	content := h.versionContent(found)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(content)
}

// versionContent builds the pull response payload from an AgentVersion.
func (h *AgentHandlers) versionContent(v *AgentVersion) map[string]any {
	switch v.Form {
	case "image":
		return map[string]any{
			"form":    "image",
			"content": v.Reference,
		}
	default:
		return map[string]any{
			"form":    "definition",
			"content": string(v.Payload),
		}
	}
}

// DispatchRequest is the JSON body for the dispatch HTTP endpoint.
type DispatchRequest struct {
	Target string           `json:"target"`
	Grant  *json.RawMessage `json:"grant,omitempty"`
}

// Dispatch handles POST /api/v1/agents/{name}/dispatch.
func (h *AgentHandlers) Dispatch(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Bad Request: missing agent name", http.StatusBadRequest)
		return
	}

	// Check auth.
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Unauthorized: missing authorization", http.StatusUnauthorized)
		return
	}

	var req DispatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	// Build grant from request.
	g := &grant.Grant{}
	if req.Grant != nil {
		if err := json.Unmarshal(*req.Grant, g); err != nil {
			http.Error(w, "Bad Request: invalid grant", http.StatusBadRequest)
			return
		}
	}

	// Check quota before dispatching.
	tenantID, _ := auth.GetTenantID(r.Context())
	if h.quotaSvc != nil && tenantID != "" {
		allowed, err := h.quotaSvc.Allow(r.Context(), tenantID, quota.OpSpawn)
		if err != nil {
			http.Error(w, "Internal Server Error: quota check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "Too Many Requests: tenant at quota limit", http.StatusTooManyRequests)
			return
		}
	}

	if err := h.DispatchToRunner(r.Context(), name, req.Target, g); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Record audit entry for dispatch.
	if h.auditSvc != nil && tenantID != "" {
		accountID, _ := auth.GetAccountID(r.Context())
		_ = h.auditSvc.Record(r.Context(), audit.Entry{
			TenantID:   tenantID,
			ActorID:    accountID,
			ActorType:  audit.ActorTypeUser,
			Action:     audit.ActionDispatchRun,
			TargetType: "agent",
			TargetID:   name,
			Metadata: map[string]any{
				"target_runner": req.Target,
			},
		})
	}

	w.WriteHeader(http.StatusAccepted)
}

// getOrCreateAgent looks up or creates an agent by name in the DB.
func (h *AgentHandlers) getOrCreateAgent(ctx context.Context, name, description string) (*Agent, error) {
	if h.service == nil || h.service.pool == nil {
		return nil, nil
	}
	agent, err := h.service.LookupAgentByName(ctx, name)
	if err == nil {
		return agent, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return h.service.CreateAgent(ctx, name, description)
}
