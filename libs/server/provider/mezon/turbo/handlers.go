// Package turbo provides the Mezon bot registry store and HTTP handlers
// for the server API. The turbo engine itself runs in the standalone
// mework-mezon-worker process, which fetches bot credentials from this API.
package turbo

import (
	"encoding/json"
	"net/http"

	"mework/libs/server/auth"
)

// BotHandlers provides HTTP handlers for the Mezon bot registry API.
// All handlers enforce tenant isolation via auth.GetTenantID(ctx).
// The turbo engine itself runs in the standalone worker process.
type BotHandlers struct {
	store *Store
}

// NewBotHandlers creates BotHandlers backed by the given store.
func NewBotHandlers(store *Store) *BotHandlers {
	return &BotHandlers{store: store}
}

// RegisterBot handles POST /api/v1/mezon/bots.
func (h *BotHandlers) RegisterBot(w http.ResponseWriter, r *http.Request) {
	tenantID, accountID := extractTenantAndAccount(r)
	if tenantID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req RegisterBotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.AppID == "" || req.APIKey == "" {
		http.Error(w, "Bad Request: app_id and api_key are required", http.StatusBadRequest)
		return
	}

	exists, err := h.store.ExistsByAppID(r.Context(), tenantID, req.AppID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "Conflict: bot with this app_id already exists", http.StatusConflict)
		return
	}

	row, err := h.store.Create(r.Context(), tenantID, accountID, &req)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, toBotResponse(row))
}

// ListBots handles GET /api/v1/mezon/bots.
func (h *BotHandlers) ListBots(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := extractTenantAndAccount(r)
	if tenantID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bots, err := h.store.ListByTenant(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	responses := make([]BotResponse, 0, len(bots))
	for _, b := range bots {
		responses = append(responses, toBotResponse(&b))
	}
	writeJSON(w, http.StatusOK, responses)
}

// GetBot handles GET /api/v1/mezon/bots/{id}.
func (h *BotHandlers) GetBot(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := extractTenantAndAccount(r)
	if tenantID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	botID := r.PathValue("id")
	if botID == "" {
		http.Error(w, "Bad Request: bot id is required", http.StatusBadRequest)
		return
	}

	row, err := h.store.Get(r.Context(), tenantID, botID)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, toBotResponse(row))
}

// DeregisterBot handles DELETE /api/v1/mezon/bots/{id}.
func (h *BotHandlers) DeregisterBot(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := extractTenantAndAccount(r)
	if tenantID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	botID := r.PathValue("id")
	if botID == "" {
		http.Error(w, "Bad Request: bot id is required", http.StatusBadRequest)
		return
	}

	if err := h.store.Delete(r.Context(), tenantID, botID); err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateBotStatus handles PATCH /api/v1/mezon/bots/{id}/status.
func (h *BotHandlers) UpdateBotStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := extractTenantAndAccount(r)
	if tenantID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	botID := r.PathValue("id")
	if botID == "" {
		http.Error(w, "Bad Request: bot id is required", http.StatusBadRequest)
		return
	}

	var req UpdateBotStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Status != "active" && req.Status != "inactive" {
		http.Error(w, "Bad Request: status must be 'active' or 'inactive'", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateStatus(r.Context(), tenantID, botID, req.Status); err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// extractTenantAndAccount returns the authenticated tenant and account IDs.
func extractTenantAndAccount(r *http.Request) (tenantID, accountID string) {
	tenantID, _ = auth.GetTenantID(r.Context())
	accountID, _ = auth.GetAccountID(r.Context())
	return
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
