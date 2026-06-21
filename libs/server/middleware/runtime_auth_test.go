package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/platform/token"
	"mework/libs/server/platform/store"
)

// newRuntimeAuthTestPool spins up a clean DB-backed pool for the rt_token
// tenant-scoping tests. Returns the pool and a server key.
func newRuntimeAuthTestPool(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed runtime auth test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { _ = store.RollbackMigrations(dsn) })

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	return ctx, pool, "supersecret"
}

// seedTenant inserts a tenant row and returns its id.
func seedTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, "INSERT INTO tenants (name) VALUES ($1) RETURNING id", name).Scan(&id); err != nil {
		t.Fatalf("seedTenant(%s): %v", name, err)
	}
	return id
}

// seedRuntimeWithToken inserts a runtime under the given tenant, with a token_lookup
// derived from rawToken, and returns the runtime id.
func seedRuntimeWithToken(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, accountID, code, rawToken, serverKey string) string {
	t.Helper()
	lookup := token.ComputeLookup(rawToken, serverKey)
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO runtimes (tenant_id, account_id, code, label, token_lookup)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, tenantID, accountID, code, "label-"+code, lookup).Scan(&id)
	if err != nil {
		t.Fatalf("seedRuntimeWithToken(%s): %v", code, err)
	}
	return id
}

// TestRuntimeAuth_AttachesTenantID realizes the delta-spec scenario
// "Runtime token required for job routes" extended for tenancy: a valid rt_token
// authenticates and the resolved identity carries its TenantID in the request
// context (GetTenantID), so downstream handlers can scope by tenant.
func TestRuntimeAuth_AttachesTenantID(t *testing.T) {
	ctx, pool, serverKey := newRuntimeAuthTestPool(t)

	var accountID string
	if err := pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Acct') RETURNING id").Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	acme := seedTenant(t, ctx, pool, "acme")

	acmeToken, err := token.GenerateRandomToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	seedRuntimeWithToken(t, ctx, pool, acme, accountID, "acme-rt", acmeToken, serverKey)

	auth := NewRuntimeAuthenticator(pool, serverKey)

	var gotTenantID string
	var gotOK bool
	target := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenantID, gotOK = GetTenantID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/v1/jobs/claim", nil)
	req.Header.Set("Authorization", "Bearer "+acmeToken)
	rec := httptest.NewRecorder()
	auth.Middleware(target).ServeHTTP(rec, req)

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
