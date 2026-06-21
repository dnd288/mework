package registry

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

func TestRegistryService(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	// Clear DB
	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	// Insert test accounts
	var accountID1 string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Account 1') RETURNING id").Scan(&accountID1)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	var accountID2 string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Account 2') RETURNING id").Scan(&accountID2)
	if err != nil {
		t.Fatalf("failed to insert account 2: %v", err)
	}

	serverKey := "supersecret"
	svc := NewService(pool, serverKey)

	// Each account lives in its own tenant; tenancy is the isolation boundary.
	tenant1, err := svc.RegisterTenant(ctx, "Tenant 1")
	if err != nil {
		t.Fatalf("failed to register tenant 1: %v", err)
	}
	tenant2, err := svc.RegisterTenant(ctx, "Tenant 2")
	if err != nil {
		t.Fatalf("failed to register tenant 2: %v", err)
	}

	// 1. Create a runtime
	rt1, tok1, err := svc.CreateRuntime(ctx, *tenant1, accountID1, "rt_code_1", "Label 1")
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}

	if tok1 == "" {
		t.Error("expected non-empty token")
	}

	if rt1.Code != "rt_code_1" || rt1.Label != "Label 1" {
		t.Errorf("unexpected runtime values: %+v", rt1)
	}

	// 2. Try duplicate code under same account (should fail with ErrDuplicateCode)
	_, _, err = svc.CreateRuntime(ctx, *tenant1, accountID1, "rt_code_1", "Label 2")
	if err != ErrDuplicateCode {
		t.Errorf("expected ErrDuplicateCode, got: %v", err)
	}

	// 3. Create same code under a different account/tenant (should succeed)
	rt2, _, err := svc.CreateRuntime(ctx, *tenant2, accountID2, "rt_code_1", "Label 2")
	if err != nil {
		t.Fatalf("failed to create same code under different account: %v", err)
	}
	if rt2.AccountID != accountID2 {
		t.Errorf("expected account ID %s, got %s", accountID2, rt2.AccountID)
	}

	// 4. List runners within tenant1
	rts, err := svc.ListRunners(ctx, *tenant1, accountID1)
	if err != nil {
		t.Fatalf("failed to list runners: %v", err)
	}

	if len(rts) != 1 {
		t.Errorf("expected 1 runtime, got %d", len(rts))
	}

	// 5. Delete runtime - cross-tenant IDOR check (deleting rt1 while scoped to
	// tenant2 must return ErrNotFound — rt1 is outside tenant2's boundary).
	err = svc.DeleteRuntime(ctx, *tenant2, accountID2, rt1.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-tenant delete, got: %v", err)
	}

	// Delete with the owning tenant should succeed
	err = svc.DeleteRuntime(ctx, *tenant1, accountID1, rt1.ID)
	if err != nil {
		t.Fatalf("failed to delete runtime: %v", err)
	}

	rts, err = svc.ListRunners(ctx, *tenant1, accountID1)
	if err != nil {
		t.Fatalf("failed to list runners: %v", err)
	}
	if len(rts) != 0 {
		t.Errorf("expected 0 runtimes, got %d", len(rts))
	}
}

// newTenancyTestService spins up a clean DB-backed Service for the tenant-scoping
// tests below. It returns the service plus an account id all seeded runtimes hang
// off of (account is an orthogonal axis; tenancy is the boundary under test).
func newTenancyTestService(t *testing.T) (context.Context, *Service, string) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
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

	// Clean every tenant-scoped table plus tenants so each test starts isolated.
	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Tenancy Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	return ctx, NewService(pool, "supersecret"), accountID
}

// TestRegisterTenant_AllocatesStableDistinctIDs realizes the delta-spec scenario
// "Operator registers a tenant": RegisterTenant(name) returns a Tenant with a
// stable, non-empty ID and the given Name, and distinct registrations get distinct
// IDs (each tenant is its own isolated namespace).
func TestRegisterTenant_AllocatesStableDistinctIDs(t *testing.T) {
	ctx, svc, _ := newTenancyTestService(t)

	tests := []struct {
		name string
	}{
		{name: "acme"},
		{name: "globex"},
		{name: "initech"},
	}

	seen := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ten, err := svc.RegisterTenant(ctx, tt.name)
			if err != nil {
				t.Fatalf("RegisterTenant(%q): %v", tt.name, err)
			}
			if ten.ID == "" {
				t.Errorf("RegisterTenant(%q): got empty ID, want stable identifier", tt.name)
			}
			if ten.Name != tt.name {
				t.Errorf("RegisterTenant(%q): Name = %q, want %q", tt.name, ten.Name, tt.name)
			}
			if seen[ten.ID] {
				t.Errorf("RegisterTenant(%q): ID %q collides with an earlier tenant; ids must be distinct", tt.name, ten.ID)
			}
			seen[ten.ID] = true
		})
	}
}

// TestListRunners_ReturnsOnlyCallersTenant realizes the delta-spec scenario
// "Listing returns only the caller's tenant": with runners under both acme and
// globex, ListRunners(ctx, acme) returns exactly acme's runners and never globex's.
func TestListRunners_ReturnsOnlyCallersTenant(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)

	acme, err := svc.RegisterTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := svc.RegisterTenant(ctx, "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	// Seed runtimes under each tenant. Codes are unique-per-account, so use distinct
	// codes; the boundary under test is tenant, not account.
	seedRuntime(t, ctx, svc.pool, acme.ID, accountID, "acme-rt-1")
	seedRuntime(t, ctx, svc.pool, acme.ID, accountID, "acme-rt-2")
	seedRuntime(t, ctx, svc.pool, globex.ID, accountID, "globex-rt-1")

	tests := []struct {
		name      string
		tenant    Tenant
		wantCount int
		wantCodes map[string]bool
	}{
		{
			name:      "acme sees only acme runners",
			tenant:    *acme,
			wantCount: 2,
			wantCodes: map[string]bool{"acme-rt-1": true, "acme-rt-2": true},
		},
		{
			name:      "globex sees only globex runners",
			tenant:    *globex,
			wantCount: 1,
			wantCodes: map[string]bool{"globex-rt-1": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.ListRunners(ctx, tt.tenant, accountID)
			if err != nil {
				t.Fatalf("ListRunners(%s): %v", tt.tenant.Name, err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("ListRunners(%s) returned %d runners, want %d: %+v", tt.tenant.Name, len(got), tt.wantCount, got)
			}
			for _, rt := range got {
				if !tt.wantCodes[rt.Code] {
					t.Errorf("ListRunners(%s) leaked runner %q from another tenant", tt.tenant.Name, rt.Code)
				}
				if rt.TenantID != tt.tenant.ID {
					t.Errorf("ListRunners(%s) returned runner %q with TenantID %q, want %q", tt.tenant.Name, rt.Code, rt.TenantID, tt.tenant.ID)
				}
			}
		})
	}
}

// TestDeleteRuntime_CrossTenantIsDenied realizes the delta-spec scenario
// "Cross-tenant access is denied": a runner owned by globex is invisible to a caller
// scoped to acme, so deleting it while scoped to acme returns ErrNotFound (invisible,
// not merely forbidden). Deleting it while scoped to its own tenant succeeds.
func TestDeleteRuntime_CrossTenantIsDenied(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)

	acme, err := svc.RegisterTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := svc.RegisterTenant(ctx, "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	globexRuntimeID := seedRuntime(t, ctx, svc.pool, globex.ID, accountID, "globex-only")

	tests := []struct {
		name    string
		tenant  Tenant
		id      string
		wantErr error
	}{
		{
			name:    "acme cannot delete globex's runner",
			tenant:  *acme,
			id:      globexRuntimeID,
			wantErr: ErrNotFound,
		},
		{
			name:    "globex can delete its own runner",
			tenant:  *globex,
			id:      globexRuntimeID,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.DeleteRuntime(ctx, tt.tenant, accountID, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DeleteRuntime(%s, %s) error = %v, want %v", tt.tenant.Name, tt.id, err, tt.wantErr)
			}
		})
	}
}

// seedRuntime inserts a runtime row directly under the given tenant and account,
// returning its id. It bypasses the service create path so the tenant-scoping tests
// don't depend on the create signature still in flux.
func seedRuntime(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, accountID, code string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO runtimes (tenant_id, account_id, code, label, token_lookup)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, tenantID, accountID, code, "label-"+code, "lookup-"+code).Scan(&id)
	if err != nil {
		t.Fatalf("seedRuntime(%s): %v", code, err)
	}
	return id
}
