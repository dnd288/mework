package mello

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseAPIError(t *testing.T) {
	body := []byte(`{"error":"validation_error","message":"bad input","fields":{"title":"required"}}`)
	err := parseAPIError(422, body)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T", err)
	}
	if !apiErr.IsValidation() {
		t.Error("422 should be validation")
	}
	if apiErr.Fields["title"] != "required" {
		t.Errorf("fields not parsed: %v", apiErr.Fields)
	}
}

func TestRateLimitedDetection(t *testing.T) {
	// By status code.
	if !(&APIError{StatusCode: 429}).IsRateLimited() {
		t.Error("429 status should be rate-limited")
	}
	// By error code even with a different status.
	if !(&APIError{StatusCode: 400, ErrorCode: "rate_limited"}).IsRateLimited() {
		t.Error("rate_limited code should be rate-limited")
	}
}

func TestExitCodeMapping(t *testing.T) {
	cases := map[error]int{
		nil:                        ExitOK,
		&APIError{StatusCode: 401}: ExitAuth,
		&APIError{StatusCode: 403}: ExitAuth,
		&APIError{StatusCode: 404}: ExitNotFound,
		&APIError{StatusCode: 422}: ExitValidation,
		&APIError{StatusCode: 500}: ExitGeneric,
	}
	for err, want := range cases {
		if got := ExitCode(err); got != want {
			t.Errorf("ExitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestGetCurrentUserDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("auth header = %q", got)
		}
		w.Write([]byte(`{"id":"u1","email":"a@b.co","name":"Ann"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok123", time.Second, "test")
	u, err := c.GetCurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "u1" || u.Name != "Ann" {
		t.Errorf("decoded user wrong: %+v", u)
	}
}

func TestNonV1BaseSwitch(t *testing.T) {
	c := NewClient("https://x/api/v1", "t", time.Second, "")
	if got := c.resolveURL("/checklist-items/1", false); got != "https://x/api/checklist-items/1" {
		t.Errorf("non-v1 switch failed: %s", got)
	}
	if got := c.resolveURL("/tickets/1", true); got != "https://x/api/v1/tickets/1" {
		t.Errorf("v1 path wrong: %s", got)
	}
}

func TestErrorResponseSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"rate_limited","message":"slow down"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", time.Second, "")
	_, err := c.ListWorkspaces()
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsRateLimited() {
		t.Fatalf("want rate-limited APIError, got %v", err)
	}
}
