package quota

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HTTPHandlers provides HTTP handlers for quota admin endpoints.
type HTTPHandlers struct {
	svc *Service
}

// NewHTTPHandlers creates quota HTTP handlers.
func NewHTTPHandlers(svc *Service) *HTTPHandlers {
	return &HTTPHandlers{svc: svc}
}

// GetLimits handles GET /api/v1/quotas/{tenant_id}.
func (h *HTTPHandlers) GetLimits(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		http.Error(w, "Bad Request: missing tenant_id", http.StatusBadRequest)
		return
	}

	lim, err := h.svc.Limits(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(lim)
}

// UpdateLimits handles PUT /api/v1/quotas/{tenant_id}.
func (h *HTTPHandlers) UpdateLimits(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		http.Error(w, "Bad Request: missing tenant_id", http.StatusBadRequest)
		return
	}

	var req Limit
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	if err := h.svc.UpsertLimits(r.Context(), tenantID, req); err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}
