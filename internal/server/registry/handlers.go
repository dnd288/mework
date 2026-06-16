package registry

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"mework/internal/server/auth"
)

type Handlers struct {
	service *Service
}

func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

type CreateRuntimeRequest struct {
	Code  string `json:"code"`
	Label string `json:"label"`
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

	rt, tok, err := h.service.CreateRuntime(r.Context(), accountID, req.Code, req.Label)
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

	runtimes, err := h.service.ListRuntimes(r.Context(), accountID)
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

	err := h.service.DeleteRuntime(r.Context(), accountID, id)
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
