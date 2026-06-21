package catalog

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

func (h *Handlers) CreateProfile(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	var req CreateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	p, err := h.service.CreateProfile(r.Context(), accountID, req)
	if err != nil {
		if errors.Is(err, ErrDuplicateName) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, ErrBodyTooLarge) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *Handlers) GetProfile(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Bad Request: missing name", http.StatusBadRequest)
		return
	}

	p, err := h.service.GetProfile(r.Context(), accountID, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *Handlers) ListProfiles(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	profiles, err := h.service.ListProfiles(r.Context(), accountID)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profiles)
}

func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Bad Request: missing name", http.StatusBadRequest)
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	p, err := h.service.UpdateProfile(r.Context(), accountID, name, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrBodyTooLarge) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *Handlers) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Bad Request: missing name", http.StatusBadRequest)
		return
	}

	err := h.service.DeleteProfile(r.Context(), accountID, name)
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
