package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
	"mework/libs/server/platform/token"
	melloprovider "mework/libs/shared/providers/mello"
)

// TestSessionRoutes_Lifecycle exercises the c0031 session lifecycle routes
// end-to-end against a real router: PAT-authed create (201)/list (200)/get
// (200)/close (204), and the runtime-authed result endpoint (204). Before the
// routes are mounted these 404; after, they behave per the delta spec.
func TestSessionRoutes_Lifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping session routes test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	const tenantID = "00000000-0000-0000-0000-000000000001"
	const serverKey = "test-server-key"

	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs; DELETE FROM account_identities; DELETE FROM runtimes;
		 DELETE FROM profiles; DELETE FROM provider_connections; DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(melloprovider.User{ID: "mello-user-1", Email: "t@e.com", Name: "T"})
	}))
	defer mockMello.Close()

	var accountID string
	if err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('A') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id, tenant_id)
		VALUES ($1, 'mello', 'mello-user-1', $2)`, accountID, tenantID); err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	// Seed a runtime whose rt_token authenticates the result endpoint.
	const rtToken = "rt-secret-token"
	lookup := token.ComputeLookup(rtToken, serverKey)
	if _, err := pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, 'wrk1', 'Worker 1', 'online', $4)`,
		"b1000000-0000-4000-a000-000000000001", tenantID, accountID, lookup); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}

	cfg := &Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		WebhookSecret:   "test-webhook-secret",
		ServerKey:       serverKey,
		MeworkSecretKey: "test-secret-key",
		MelloBaseURL:    mockMello.URL,
	}
	srv := NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	const pat = "valid-user-pat-token"

	do := func(method, path, body, auth string) *http.Response {
		req, _ := http.NewRequest(method, httpSrv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+auth)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}

	// Create → 201 with a session id. Dispatch is best-effort to a runner that
	// isn't subscribed; the in-memory broker still accepts the publish.
	resp := do(http.MethodPost, "/api/v1/sessions",
		`{"agent_name":"code-fixer","runner":"b1000000-0000-4000-a000-000000000001"}`, pat)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		ID     string `json:"id"`
		Owner  string `json:"owner"`
		Tenant string `json:"tenant"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == "" {
		t.Fatal("create returned empty session id")
	}
	if created.Owner != accountID {
		t.Errorf("owner = %q, want %q (from PAT)", created.Owner, accountID)
	}
	if created.Tenant != tenantID {
		t.Errorf("tenant = %q, want %q (from PAT)", created.Tenant, tenantID)
	}

	// List → 200 and includes the session.
	resp = do(http.MethodGet, "/api/v1/sessions", "", pat)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var list []struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	found := false
	for _, s := range list {
		if s.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("list did not include created session %q: %+v", created.ID, list)
	}

	// Get → 200.
	resp = do(http.MethodGet, "/api/v1/sessions/"+created.ID, "", pat)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Result endpoint (runtime-authed) → 204.
	resp = do(http.MethodPost, "/api/v1/runners/sessions/"+created.ID+"/result",
		`{"status":"done","summary":"ok"}`, rtToken)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("result status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Result endpoint rejects a PAT (wrong authenticator) → 401.
	resp = do(http.MethodPost, "/api/v1/runners/sessions/"+created.ID+"/result",
		`{"status":"done"}`, pat)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("result with PAT status = %d, want 401 (must be runtime-authed)", resp.StatusCode)
	}
	resp.Body.Close()

	// Close → 204.
	resp = do(http.MethodDelete, "/api/v1/sessions/"+created.ID, "", pat)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("close status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()
}
