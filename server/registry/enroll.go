package registry

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mework/server/auth"
)

type issueRegistrationTokenRequest struct {
	TenantID string `json:"tenant_id"`
}

type issueRegistrationTokenResponse struct {
	Token string `json:"token"`
}

func (h *Handlers) IssueRegistrationToken(w http.ResponseWriter, r *http.Request) {
	accountID, ok := auth.GetAccountID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing account ID", http.StatusUnauthorized)
		return
	}
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: missing tenant ID", http.StatusUnauthorized)
		return
	}

	var req issueRegistrationTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid body", http.StatusBadRequest)
		return
	}

	if req.TenantID == "" {
		http.Error(w, "Bad Request: tenant_id is required", http.StatusBadRequest)
		return
	}

	if req.TenantID != tenantID {
		http.Error(w, "Forbidden: tenant mismatch", http.StatusForbidden)
		return
	}

	rawToken, err := h.service.IssueRegistrationToken(r.Context(), Tenant{ID: tenantID}, WithAccountID(accountID))
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(issueRegistrationTokenResponse{Token: rawToken})
}

type enrollRunnerResponse struct {
	RunnerID string `json:"runner_id"`
	Secret   string `json:"secret"`
}

func (h *Handlers) EnrollRunner(w http.ResponseWriter, r *http.Request) {
	regToken, err := extractBearerToken(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	rt, secret, err := h.service.ConsumeAndEnrollRunner(r.Context(), regToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRegistrationToken) {
			http.Error(w, "Unauthorized: invalid registration token", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(enrollRunnerResponse{
		RunnerID: rt.ID,
		Secret:   secret,
	})
}

func extractBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("missing Authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", errors.New("invalid Authorization header format")
	}

	token := parts[1]
	if token == "" {
		return "", errors.New("empty token")
	}
	return token, nil
}
