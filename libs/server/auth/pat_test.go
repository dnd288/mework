package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/shared/providers/mello"
	"mework/libs/server/platform/store"
)

func TestPATAuthMiddleware(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed PAT authenticator test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialize test database
	err := store.RunMigrations(dsn)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	defer func() {
		_ = store.RollbackMigrations(dsn)
	}()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer pool.Close()

	// Clean up database tables
	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM accounts;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	// 1. Setup mock Mello server
	melloCallCount := 0
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		melloCallCount++
		token := r.Header.Get("Authorization")
		if token != "Bearer mello_pat_valid" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(mello.APIError{
				StatusCode: http.StatusUnauthorized,
				ErrorCode:  "unauthorized",
				Message:    "Invalid token",
			})
			return
		}

		switch r.URL.Path {
		case "/me":
			user := mello.User{
				ID:    "mello_user_123",
				Email: "user@example.com",
				Name:  "Test Mello User",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(user)
		case "/workspaces":
			wps := []mello.Workspace{
				{ID: "ws_1", Name: "Workspace 1"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(wps)
		case "/workspaces/ws_1/boards":
			boards := []mello.Board{
				{ID: "board_abc", Name: "Board ABC"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(boards)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockMello.Close()

	authenticator := NewPATAuthenticator(pool, mockMello.URL)
	authenticator.TTL = 500 * time.Millisecond // Use short TTL for testing caching

	// Set up a simple target handler that returns context values
	targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := GetAccountID(r.Context())
		if !ok {
			t.Error("context missing account ID")
		}
		token, ok := GetPATToken(r.Context())
		if !ok {
			t.Error("context missing token")
		}

		w.Header().Set("X-Account-ID", accountID)
		w.Header().Set("X-PAT-Token", token)
		w.WriteHeader(http.StatusOK)
	})

	middleware := authenticator.Middleware(targetHandler)

	// Test case 1: Request without Auth header -> 401
	{
		req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	}

	// Test case 2: Request with invalid Authorization format -> 401
	{
		req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
		req.Header.Set("Authorization", "InvalidFormat token")
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	}

	// Test case 3: Request with invalid token -> 401
	{
		req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
		req.Header.Set("Authorization", "Bearer mello_pat_invalid")
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	}

	// Test case 4: Valid token -> 200, context populated, db entry created
	var firstAccountID string
	{
		req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
		req.Header.Set("Authorization", "Bearer mello_pat_valid")
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
		}

		firstAccountID = rec.Header().Get("X-Account-ID")
		if firstAccountID == "" {
			t.Fatal("expected non-empty account ID")
		}

		if rec.Header().Get("X-PAT-Token") != "mello_pat_valid" {
			t.Errorf("expected token in header to match, got %s", rec.Header().Get("X-PAT-Token"))
		}

		// Wait briefly for the async container sync goroutine to finish
		time.Sleep(100 * time.Millisecond)

		// Assert db record exists
		var extUserID string
		err = pool.QueryRow(ctx, "SELECT external_user_id FROM account_identities WHERE account_id = $1", firstAccountID).Scan(&extUserID)
		if err != nil {
			t.Fatalf("failed to query identity: %v", err)
		}
		if extUserID != "mello_user_123" {
			t.Errorf("expected external user ID mello_user_123, got %s", extUserID)
		}

		// Verify watched container synced
		var count int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM watched_containers WHERE account_id = $1 AND external_container_id = $2", firstAccountID, "board_abc").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query watched container: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 watched container, got %d", count)
		}
	}

	// Test case 5: Caching logic (second request should not hit Mello server)
	melloCallCountBefore := melloCallCount
	{
		req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
		req.Header.Set("Authorization", "Bearer mello_pat_valid")
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		if rec.Header().Get("X-Account-ID") != firstAccountID {
			t.Errorf("expected cached account ID %s, got %s", firstAccountID, rec.Header().Get("X-Account-ID"))
		}

		if melloCallCount != melloCallCountBefore {
			t.Errorf("expected call count to remain %d, but incremented to %d (cache hit failed)", melloCallCountBefore, melloCallCount)
		}
	}

	// Test case 6: Caching expiry
	time.Sleep(600 * time.Millisecond)
	{
		req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
		req.Header.Set("Authorization", "Bearer mello_pat_valid")
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		if melloCallCount == melloCallCountBefore {
			t.Error("expected mock server to be called after cache expiry, but call count didn't increase")
		}
	}
}

// mockMelloUser stands up a Mello stub that authenticates a single PAT and returns a
// fixed user, with no boards (so the container sync is a no-op). It returns the URL.
func mockMelloUser(t *testing.T, validPAT, userID string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+validPAT {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(mello.APIError{
				StatusCode: http.StatusUnauthorized,
				ErrorCode:  "unauthorized",
				Message:    "Invalid token",
			})
			return
		}
		switch r.URL.Path {
		case "/me":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mello.User{ID: userID, Email: userID + "@example.com", Name: "User " + userID})
		case "/workspaces":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]mello.Workspace{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestPATAuth_AttachesTenantID realizes the delta-spec scenario "Credential is bound
// to its tenant" (specs/auth-and-secrets/spec.md) for the PAT credential: an
// authenticated PAT resolves the tenant that owns its account and exposes it in the
// request context via GetTenantID, so management routes can scope by tenant.
func TestPATAuth_AttachesTenantID(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed PAT tenancy test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM accounts; DELETE FROM tenants;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	// A dedicated tenant owns the account the PAT maps to.
	var acme string
	if err := pool.QueryRow(ctx, "INSERT INTO tenants (name) VALUES ('acme') RETURNING id").Scan(&acme); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	var accountID string
	if err := pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Acme Acct') RETURNING id").Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id, tenant_id)
		VALUES ($1, 'mello', 'mello_user_acme', $2)
	`, accountID, acme); err != nil {
		t.Fatalf("insert identity: %v", err)
	}

	authn := NewPATAuthenticator(pool, mockMelloUser(t, "pat_acme", "mello_user_acme"))

	var gotTenantID string
	var gotOK bool
	target := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenantID, gotOK = GetTenantID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/v1/runtimes", nil)
	req.Header.Set("Authorization", "Bearer pat_acme")
	rec := httptest.NewRecorder()
	authn.Middleware(target).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
	if !gotOK {
		t.Fatal("context missing tenant ID")
	}
	if gotTenantID != acme {
		t.Errorf("GetTenantID = %q, want acme %q", gotTenantID, acme)
	}
}
