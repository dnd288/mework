package scheduler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mework/libs/server/auth"
	"mework/libs/shared/core"
)

// createScheduleRequest is the JSON body for creating a schedule.
type createScheduleRequest struct {
	Kind   core.ScheduleKind  `json:"kind"`
	Cron   string             `json:"cron,omitempty"`
	Every  string             `json:"every,omitempty"`
	At     string             `json:"at,omitempty"`
	TZ     string             `json:"tz,omitempty"`
	Agent  string             `json:"agent"`
	Target string             `json:"target"`
	Grant  []byte             `json:"grant,omitempty"`
	Missed core.MissedPolicy  `json:"missed,omitempty"`
}

// createScheduleResponse is the JSON response for a created schedule.
type createScheduleResponse struct {
	ID string `json:"id"`
}

// listScheduleResponse is the JSON response for listing schedules.
type listScheduleResponse struct {
	IDs []string `json:"ids"`
}

// getScheduleResponse is the JSON response for getting a schedule.
type getScheduleResponse struct {
	Spec  core.ScheduleSpec  `json:"spec"`
	State core.ScheduleState `json:"state"`
}

// Handlers holds HTTP handlers for scheduler operations.
type Handlers struct {
	service *Service
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{service: svc}
}

// CreateSchedule handles POST /api/v1/schedules.
func (h *Handlers) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant", http.StatusUnauthorized)
		return
	}

	var req createScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	spec := core.ScheduleSpec{
		Kind:   req.Kind,
		Cron:   req.Cron,
		Every:  req.Every,
		At:     req.At,
		TZ:     req.TZ,
		Agent:  req.Agent,
		Target: req.Target,
		Grant:  req.Grant,
		Missed: req.Missed,
	}

	id, err := h.service.Schedule(r.Context(), tenantID, spec)
	if err != nil {
		if errors.Is(err, ErrInvalidSpec) {
			http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(createScheduleResponse{ID: id})
}

// PauseSchedule handles POST /api/v1/schedules/{id}/pause.
func (h *Handlers) PauseSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant", http.StatusUnauthorized)
		return
	}

	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		http.Error(w, "Bad Request: missing schedule ID", http.StatusBadRequest)
		return
	}

	if err := h.service.Pause(r.Context(), tenantID, scheduleID); err != nil {
		if errors.Is(err, ErrScheduleNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ResumeSchedule handles POST /api/v1/schedules/{id}/resume.
func (h *Handlers) ResumeSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant", http.StatusUnauthorized)
		return
	}

	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		http.Error(w, "Bad Request: missing schedule ID", http.StatusBadRequest)
		return
	}

	if err := h.service.Resume(r.Context(), tenantID, scheduleID); err != nil {
		if errors.Is(err, ErrScheduleNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// CancelSchedule handles POST /api/v1/schedules/{id}/cancel.
func (h *Handlers) CancelSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant", http.StatusUnauthorized)
		return
	}

	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		http.Error(w, "Bad Request: missing schedule ID", http.StatusBadRequest)
		return
	}

	if err := h.service.Cancel(r.Context(), tenantID, scheduleID); err != nil {
		if errors.Is(err, ErrScheduleNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ListSchedules handles GET /api/v1/schedules.
func (h *Handlers) ListSchedules(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant", http.StatusUnauthorized)
		return
	}

	ids, err := h.service.List(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(listScheduleResponse{IDs: ids})
}

// GetSchedule handles GET /api/v1/schedules/{id}.
func (h *Handlers) GetSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant", http.StatusUnauthorized)
		return
	}

	scheduleID := chi.URLParam(r, "id")
	if scheduleID == "" {
		http.Error(w, "Bad Request: missing schedule ID", http.StatusBadRequest)
		return
	}

	spec, state, err := h.service.Get(r.Context(), tenantID, scheduleID)
	if err != nil {
		if errors.Is(err, ErrScheduleNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(getScheduleResponse{Spec: *spec, State: state})
}
