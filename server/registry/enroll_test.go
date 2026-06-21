package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mework/server/auth"
)

func withRequestBody(method, target, accountID, tenantID string, body []byte) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), auth.AccountIDKey, accountID)
	ctx = context.WithValue(ctx, auth.TenantIDKey, tenantID)
	return req.WithContext(ctx)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func TestEnrollServer_Success(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)
	tenant, err := svc.RegisterTenant(ctx, "enroll-success")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	handlers := NewHandlers(svc)

	issueBody := mustMarshal(t, map[string]string{"tenant_id": tenant.ID})
	req := withRequestBody(http.MethodPost, "/api/v1/runners/registration-tokens",
		accountID, tenant.ID, issueBody)
	rec := httptest.NewRecorder()
	handlers.IssueRegistrationToken(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("IssueRegistrationToken status = %d, want 201 (body: %s)",
			rec.Code, rec.Body.String())
	}

	var issueResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&issueResp); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	if issueResp.Token == "" {
		t.Fatal("expected non-empty registration token")
	}

	exchangeBody := mustMarshal(t, map[string]string{"token": issueResp.Token})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll",
		bytes.NewReader(exchangeBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+issueResp.Token)
	rec2 := httptest.NewRecorder()
	handlers.EnrollRunner(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("EnrollRunner status = %d, want 200 (body: %s)",
			rec2.Code, rec2.Body.String())
	}

	var enrollResp struct {
		RunnerID string `json:"runner_id"`
		Secret   string `json:"secret"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&enrollResp); err != nil {
		t.Fatalf("decode enroll response: %v", err)
	}
	if enrollResp.RunnerID == "" {
		t.Error("expected non-empty runner_id")
	}
	if enrollResp.Secret == "" {
		t.Error("expected non-empty secret")
	}

	runners, err := svc.ListRunners(ctx, *tenant, accountID)
	if err != nil {
		t.Fatalf("ListRunners: %v", err)
	}
	found := false
	for _, r := range runners {
		if r.ID == enrollResp.RunnerID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("runner %q not found in ListRunners for tenant %q",
			enrollResp.RunnerID, tenant.ID)
	}
}

func TestEnrollServer_ExpiredToken(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)
	tenant, err := svc.RegisterTenant(ctx, "enroll-expiry")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	handlers := NewHandlers(svc)

	issueBody := mustMarshal(t, map[string]string{"tenant_id": tenant.ID})
	req := withRequestBody(http.MethodPost, "/api/v1/runners/registration-tokens",
		accountID, tenant.ID, issueBody)
	rec := httptest.NewRecorder()
	handlers.IssueRegistrationToken(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("IssueRegistrationToken status = %d, want 201", rec.Code)
	}

	var issueResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&issueResp); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}

	time.Sleep(2 * time.Second)

	exchangeBody := mustMarshal(t, map[string]string{"token": issueResp.Token})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll",
		bytes.NewReader(exchangeBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+issueResp.Token)
	rec2 := httptest.NewRecorder()
	handlers.EnrollRunner(rec2, req2)

	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("EnrollRunner status = %d, want 401 for expired token (body: %s)",
			rec2.Code, rec2.Body.String())
	}
}

func TestEnrollServer_InvalidToken(t *testing.T) {
	_, svc, _ := newTenancyTestService(t)
	handlers := NewHandlers(svc)

	exchangeBody := mustMarshal(t, map[string]string{"token": "nonexistent-reg-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll",
		bytes.NewReader(exchangeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer nonexistent-reg-token")
	rec := httptest.NewRecorder()
	handlers.EnrollRunner(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("EnrollRunner status = %d, want 401 for invalid token (body: %s)",
			rec.Code, rec.Body.String())
	}
}

func TestEnrollServer_SingleUse(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)
	tenant, err := svc.RegisterTenant(ctx, "enroll-singleuse")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}
	handlers := NewHandlers(svc)

	issueBody := mustMarshal(t, map[string]string{"tenant_id": tenant.ID})
	req := withRequestBody(http.MethodPost, "/api/v1/runners/registration-tokens",
		accountID, tenant.ID, issueBody)
	rec := httptest.NewRecorder()
	handlers.IssueRegistrationToken(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("IssueRegistrationToken status = %d, want 201", rec.Code)
	}

	var issueResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&issueResp); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}

	// First use — should succeed.
	exchangeBody := mustMarshal(t, map[string]string{"token": issueResp.Token})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/runners/enroll",
		bytes.NewReader(exchangeBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+issueResp.Token)
	rec2 := httptest.NewRecorder()
	handlers.EnrollRunner(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("first EnrollRunner status = %d, want 200 (body: %s)",
			rec2.Code, rec2.Body.String())
	}

	// Second use with same token — must fail (consumed).
	rec3 := httptest.NewRecorder()
	handlers.EnrollRunner(rec3, req2.Clone(req2.Context()))

	if rec3.Code != http.StatusConflict && rec3.Code != http.StatusUnauthorized {
		t.Errorf("second EnrollRunner status = %d, want 409 or 401 (body: %s)",
			rec3.Code, rec3.Body.String())
	}
}

func TestEnrollServer_EndpointAuth(t *testing.T) {
	_, svc, _ := newTenancyTestService(t)
	handlers := NewHandlers(svc)

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		method  string
		target  string
		body    []byte
		want    int
	}{
		{
			name:    "IssueRegistrationToken without PAT gets 401",
			handler: handlers.IssueRegistrationToken,
			method:  http.MethodPost,
			target:  "/api/v1/runners/registration-tokens",
			body:    mustMarshal(t, map[string]string{"tenant_id": "fake"}),
			want:    http.StatusUnauthorized,
		},
		{
			name:    "EnrollRunner without auth gets 401",
			handler: handlers.EnrollRunner,
			method:  http.MethodPost,
			target:  "/api/v1/runners/enroll",
			body:    mustMarshal(t, map[string]string{"token": "fake"}),
			want:    http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			tt.handler(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body: %s)",
					rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
