package audit

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// HTTPHandlers provides HTTP handlers for audit log endpoints.
type HTTPHandlers struct {
	svc *Service
}

// NewHTTPHandlers creates audit HTTP handlers.
func NewHTTPHandlers(svc *Service) *HTTPHandlers {
	return &HTTPHandlers{svc: svc}
}

// QueryLog handles GET /api/v1/audit/{tenant_id}.
func (h *HTTPHandlers) QueryLog(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		http.Error(w, "Bad Request: missing tenant_id", http.StatusBadRequest)
		return
	}

	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	entries, err := h.svc.Query(r.Context(), tenantID, limit)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}
