package connection

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"mework/libs/server/auth"
)

type Handlers struct {
	service *Service
}

func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

func (h *Handlers) CreateConnection(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	var req CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	conn, err := h.service.CreateConnection(r.Context(), accountID, req.ProviderCode, req.ProviderToken, req.WebhookSecret, req.Config)
	if err != nil {
		if errors.Is(err, ErrDuplicateConnection) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(conn)
}

func (h *Handlers) GetConnection(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	providerCode := chi.URLParam(r, "provider_code")
	if providerCode == "" {
		http.Error(w, "Bad Request: missing provider_code", http.StatusBadRequest)
		return
	}

	conn, err := h.service.GetConnection(r.Context(), accountID, providerCode)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conn)
}

func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	conns, err := h.service.ListConnections(r.Context(), accountID)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conns)
}

func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	providerCode := chi.URLParam(r, "provider_code")
	if providerCode == "" {
		http.Error(w, "Bad Request: missing provider_code", http.StatusBadRequest)
		return
	}

	err := h.service.DeleteConnection(r.Context(), accountID, providerCode)
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
