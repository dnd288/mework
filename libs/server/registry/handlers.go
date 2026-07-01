package registry

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"mework/libs/server/audit"
	"mework/libs/server/auth"
)

type Handlers struct {
	service  *Service
	auditSvc *audit.Service
}

func NewHandlers(service *Service, auditSvc *audit.Service) *Handlers {
	return &Handlers{service: service, auditSvc: auditSvc}
}

// callerTenant returns the tenant of the authenticated credential, which the auth
// middleware resolves into the request context. Every tenant-scoped handler routes
// its service call through this so the production HTTP path is scoped by the caller's
// own tenant (not a hardcoded default), enforcing cross-tenant isolation at the route
// layer. It falls back to the default tenant only when the context carries no tenant
// (e.g. a legacy single-tenant credential during migration).
func callerTenant(r *http.Request) Tenant {
	if tid, ok := auth.GetTenantID(r.Context()); ok && tid != "" {
		return Tenant{ID: tid}
	}
	return Tenant{ID: DefaultTenantID}
}

type CreateRuntimeRequest struct {
	Code  string   `json:"code"`
	Label string   `json:"label"`
	Specs []string `json:"specs,omitempty"`
}

type CreateRuntimeResponse struct {
	Runtime
	Token string `json:"token"`
}

func (h *Handlers) CreateRuntime(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	var req CreateRuntimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	rt, tok, err := h.service.CreateRuntime(r.Context(), callerTenant(r), accountID, req.Code, req.Label, req.Specs...)
	if err != nil {
		if errors.Is(err, ErrDuplicateCode) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(CreateRuntimeResponse{
		Runtime: *rt,
		Token:   tok,
	})
}

func (h *Handlers) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	runtimes, err := h.service.ListRunners(r.Context(), callerTenant(r), accountID)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runtimes)
}

func (h *Handlers) DeleteRuntime(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Bad Request: missing runtime ID", http.StatusBadRequest)
		return
	}

	err := h.service.DeleteRuntime(r.Context(), callerTenant(r), accountID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
