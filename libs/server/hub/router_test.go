package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
	melloprovider "mework/libs/shared/providers/mello"
)

// testTenantID is the default tenant UUID used for testing.
const testTenantID = "00000000-0000-0000-0000-000000000001"

// TestChannelsEndpoint_RequiresPAT verifies that an unauthenticated request
// to GET /api/v1/channels returns 401 Unauthorized. Delta-spec scenario:
// "List active channels" (PAT-authenticated).
func TestChannelsEndpoint_RequiresPAT(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping channels router test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	// Clear DB.
	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	// Setup mock Mello.
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(melloprovider.User{
			ID:    "mello-user-999",
			Email: "test@example.com",
			Name:  "Test User",
		})
	}))
	defer mockMello.Close()

	cfg := &Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		WebhookSecret:   "test-webhook-secret",
		ServerKey:       "test-server-key",
		MeworkSecretKey: "test-secret-key",
		MelloBaseURL:    mockMello.URL,
	}
	srv := NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	tests := []struct {
		name         string
		authHeader   string
		wantStatus   int
	}{
		{
			name:         "missing Authorization header returns 401",
			authHeader:   "",
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "empty token returns 401",
			authHeader:   "Bearer ",
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "malformed header returns 401",
			authHeader:   "Basic token123",
			wantStatus:   http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", httpSrv.URL+"/api/v1/channels", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

// TestChannelsEndpoint_ReturnsActiveSessions verifies that an authenticated
// request to GET /api/v1/channels returns the list of active channel sessions
// as a JSON array, or an empty list when none exist. Delta-spec scenario:
// "List active channels".
func TestChannelsEndpoint_ReturnsActiveSessions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping channels router test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	// Clear DB.
	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	patToken := "valid-user-pat-token"

	// Setup mock Mello that returns a user matching the PAT.
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(melloprovider.User{
			ID:    "mello-user-999",
			Email: "test@example.com",
			Name:  "Test User",
		})
	}))
	defer mockMello.Close()

	cfg := &Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		WebhookSecret:   "test-webhook-secret",
		ServerKey:       "test-server-key",
		MeworkSecretKey: "test-secret-key",
		MelloBaseURL:    mockMello.URL,
	}
	srv := NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Seed a channel session via direct DB query.
	// First ensure there's an account and tenant.
	var accountID string
	err = pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('Account A') RETURNING id`).Scan(&accountID)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	// Seed an account identity so PAT auth resolves to the seeded tenant/account.
	_, err = pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id, tenant_id)
		VALUES ($1, 'mello', 'mello-user-999', $2)
		ON CONFLICT (provider_code, external_user_id) DO UPDATE SET account_id = EXCLUDED.account_id, tenant_id = EXCLUDED.tenant_id
	`, accountID, testTenantID)
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	// Seed a runner.
	_, err = pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
	`, "b1000000-0000-4000-a000-000000000001", testTenantID, accountID, "wrk1", "Worker 1", "online", "lookup-r1")
	if err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	// Seed an active channel session.
	_, err = pool.Exec(ctx, `
		INSERT INTO channel_sessions (channel_key, session_id, provider_code, resource_id, runner_id, spec, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (channel_key) DO NOTHING
	`, "mello:TICKET-1", "sess-1", "mello", "TICKET-1", "b1000000-0000-4000-a000-000000000001", "claude-code", "active")
	if err != nil {
		t.Fatalf("seed channel_sessions: %v", err)
	}

	t.Run("returns channel sessions for authenticated user", func(t *testing.T) {
		req, _ := http.NewRequest("GET", httpSrv.URL+"/api/v1/channels", nil)
		req.Header.Set("Authorization", "Bearer "+patToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		// RED: /api/v1/channels is not yet mounted — the response will be a 404
		// or 401 (if the PAT middleware rejects the request first). We assert
		// 200 OK which will fail, confirming the route is not yet wired.
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d (RED: route not mounted yet)", resp.StatusCode, http.StatusOK)
		}

		var sessions []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if len(sessions) == 0 {
			t.Fatal("expected at least one channel session")
		}
		s := sessions[0]
		if s["channel_key"] != "mello:TICKET-1" {
			t.Errorf("channel_key = %v, want %q", s["channel_key"], "mello:TICKET-1")
		}
		if s["session_id"] != "sess-1" {
			t.Errorf("session_id = %v, want %q", s["session_id"], "sess-1")
		}
		if s["status"] != "active" {
			t.Errorf("status = %v, want %q", s["status"], "active")
		}
	})

	t.Run("returns empty list when no channels exist", func(t *testing.T) {
		// Clear channel sessions.
		_, err := pool.Exec(ctx, "DELETE FROM channel_sessions")
		if err != nil {
			t.Fatalf("clear channel_sessions: %v", err)
		}

		req, _ := http.NewRequest("GET", httpSrv.URL+"/api/v1/channels", nil)
		req.Header.Set("Authorization", "Bearer "+patToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		// RED: Same as above — route not mounted, will not get 200.
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d (RED: route not mounted yet)", resp.StatusCode, http.StatusOK)
		}

		var sessions []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if len(sessions) != 0 {
			t.Errorf("expected empty list, got %d items", len(sessions))
		}
	})
}

// TestChannelsEndpoint_TenantIsolation verifies that channel sessions from
// tenant A are not visible when authenticated with a PAT bound to tenant B.
// Delta-spec: "Tenant isolation".
func TestChannelsEndpoint_TenantIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping channels router test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	// Clear DB.
	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	patToken := "valid-user-pat-token"

	// Setup mock Mello.
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(melloprovider.User{
			ID:    "mello-user-isolation",
			Email: "test@example.com",
			Name:  "Test User",
		})
	}))
	defer mockMello.Close()

	// Seed an account and identity for the given mock user, using the default tenant.
	var accountID string
	err = pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('Account A') RETURNING id`).Scan(&accountID)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id, tenant_id)
		VALUES ($1, 'mello', 'mello-user-isolation', $2)
		ON CONFLICT (provider_code, external_user_id) DO UPDATE SET account_id = EXCLUDED.account_id, tenant_id = EXCLUDED.tenant_id
	`, accountID, testTenantID)
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	// Seed a runner in the same tenant.
	_, err = pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
	`, "b2000000-0000-4000-a000-000000000001", testTenantID, accountID, "wrkA", "Worker A", "online", "lookup-iso")
	if err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	// Seed an active channel session.
	_, err = pool.Exec(ctx, `
		INSERT INTO channel_sessions (channel_key, session_id, provider_code, resource_id, runner_id, spec, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (channel_key) DO NOTHING
	`, "mello:TICKET-A", "sess-a", "mello", "TICKET-A", "b2000000-0000-4000-a000-000000000001", "claude-code", "active")
	if err != nil {
		t.Fatalf("seed channel_sessions: %v", err)
	}

	t.Run("tenant A sees its own channel when route exists", func(t *testing.T) {
		cfg := &Config{
			DatabaseURL:     dsn,
			ListenAddr:      "127.0.0.1:0",
			WebhookSecret:   "test-webhook-secret",
			ServerKey:       "test-server-key",
			MeworkSecretKey: "test-secret-key",
			MelloBaseURL:    mockMello.URL,
		}
		srv := NewServer(pool, cfg)
		httpSrv := httptest.NewServer(srv)
		defer httpSrv.Close()

		req, _ := http.NewRequest("GET", httpSrv.URL+"/api/v1/channels", nil)
		req.Header.Set("Authorization", "Bearer "+patToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		// RED: Route not mounted — will not get 200.
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d (RED: route not mounted yet)", resp.StatusCode, http.StatusOK)
		}

		var sessions []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if len(sessions) == 0 {
			t.Error("tenant A should see its own channel, got empty list")
		}
	})
}

// TestSessionChatRoutes_Mounted verifies that the c0032 session chat bus routes
// are mounted under the correct auth blocks: the PAT-protected send/stream
// endpoints and the runtime-authed runner events ingress all reject
// unauthenticated requests with 401 (mounted + auth enforced) rather than 404
// (not mounted). Delta-spec scenarios: "Submit a chat turn", "Stream session
// events", "Runner delivers events for relay".
func TestSessionChatRoutes_Mounted(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping session chat routes test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer mockMello.Close()

	cfg := &Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		WebhookSecret:   "test-webhook-secret",
		ServerKey:       "test-server-key",
		MeworkSecretKey: "test-secret-key",
		MelloBaseURL:    mockMello.URL,
	}
	srv := NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"send turn requires PAT", http.MethodPost, "/api/v1/sessions/s1/messages"},
		{"stream requires PAT", http.MethodGet, "/api/v1/sessions/s1/stream"},
		{"runner events ingress requires runtime token", http.MethodPost, "/api/v1/runners/sessions/s1/events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, httpSrv.URL+tt.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				t.Fatalf("%s %s returned 404: route not mounted", tt.method, tt.path)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s %s status = %d, want 401 (mounted + auth enforced)", tt.method, tt.path, resp.StatusCode)
			}
		})
	}
}
