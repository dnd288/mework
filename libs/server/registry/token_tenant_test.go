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

// newTokenTenantTestService spins up a clean DB-backed Service for the
// registration-token tenant-binding tests. It returns the service plus an account
// id every enrolled runner hangs off of (account is orthogonal; tenancy is the
// boundary under test).
func newTokenTenantTestService(t *testing.T) (context.Context, *Service, string) {
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

	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Token Tenancy Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	return ctx, NewService(pool, "supersecret"), accountID
}

// TestIssueRegistrationToken_RecordsOwningTenant realizes the delta-spec scenario
// "Issued token is bound to its tenant" (specs/tenancy/spec.md): issuing a
// registration token for tenant acme records acme as its owning tenant. The
// returned plaintext token is non-empty and its stored record's TenantID equals the
// issuing tenant; distinct tenants get distinct tokens bound to their own ids.
func TestIssueRegistrationToken_RecordsOwningTenant(t *testing.T) {
	ctx, svc, _ := newTokenTenantTestService(t)

	acme, err := svc.RegisterTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := svc.RegisterTenant(ctx, "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	tests := []struct {
		name   string
		tenant Tenant
	}{
		{name: "acme token bound to acme", tenant: *acme},
		{name: "globex token bound to globex", tenant: *globex},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawToken, err := svc.IssueRegistrationToken(ctx, tt.tenant)
			if err != nil {
				t.Fatalf("IssueRegistrationToken(%s): %v", tt.tenant.Name, err)
			}
			if rawToken == "" {
				t.Fatalf("IssueRegistrationToken(%s): got empty token", tt.tenant.Name)
			}

			rec, err := svc.LookupRegistrationToken(ctx, rawToken)
			if err != nil {
				t.Fatalf("LookupRegistrationToken(%s): %v", tt.tenant.Name, err)
			}
			if rec.TenantID != tt.tenant.ID {
				t.Errorf("registration token TenantID = %q, want %q", rec.TenantID, tt.tenant.ID)
			}
		})
	}
}

// TestEnrollWithRegistrationToken_YieldsTenantBoundIdentity realizes the delta-spec
// scenario "Enrolling yields a tenant-bound identity": a runner enrolled with a
// registration token issued for acme yields a runner identity bound to acme
// (rt.TenantID == acme). A token issued for globex enrolls under globex.
func TestEnrollWithRegistrationToken_YieldsTenantBoundIdentity(t *testing.T) {
	ctx, svc, accountID := newTokenTenantTestService(t)

	acme, err := svc.RegisterTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := svc.RegisterTenant(ctx, "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	acmeToken, err := svc.IssueRegistrationToken(ctx, *acme)
	if err != nil {
		t.Fatalf("IssueRegistrationToken(acme): %v", err)
	}
	globexToken, err := svc.IssueRegistrationToken(ctx, *globex)
	if err != nil {
		t.Fatalf("IssueRegistrationToken(globex): %v", err)
	}

	tests := []struct {
		name       string
		token      string
		code       string
		wantTenant string
	}{
		{name: "acme token enrolls under acme", token: acmeToken, code: "acme-enrolled", wantTenant: acme.ID},
		{name: "globex token enrolls under globex", token: globexToken, code: "globex-enrolled", wantTenant: globex.ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := svc.EnrollRunner(ctx, tt.token, accountID, tt.code, "Label "+tt.code)
			if err != nil {
				t.Fatalf("EnrollRunner(%s): %v", tt.name, err)
			}
			if rt.TenantID != tt.wantTenant {
				t.Errorf("enrolled runner TenantID = %q, want %q", rt.TenantID, tt.wantTenant)
			}
		})
	}
}

// TestEnrollWithRegistrationToken_RejectsCrossTenant realizes the delta-spec
// requirement "Registration tokens are scoped to a tenant": a token issued for acme
// MUST NOT enroll a runner into any other tenant, and an unknown/invalid token is
// rejected. The enrolled identity always inherits the token's tenant, never a
// caller-supplied one, so cross-tenant enrollment is denied by construction.
func TestEnrollWithRegistrationToken_RejectsCrossTenant(t *testing.T) {
	ctx, svc, accountID := newTokenTenantTestService(t)

	acme, err := svc.RegisterTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := svc.RegisterTenant(ctx, "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	acmeToken, err := svc.IssueRegistrationToken(ctx, *acme)
	if err != nil {
		t.Fatalf("IssueRegistrationToken(acme): %v", err)
	}

	// An acme-issued token enrolls a runner; that runner must land in acme, never globex.
	rt, err := svc.EnrollRunner(ctx, acmeToken, accountID, "acme-via-token", "Label")
	if err != nil {
		t.Fatalf("EnrollRunner(acme token): %v", err)
	}
	if rt.TenantID == globex.ID {
		t.Fatalf("acme-issued token enrolled a runner into globex; cross-tenant enrollment must be denied")
	}
	if rt.TenantID != acme.ID {
		t.Errorf("enrolled runner TenantID = %q, want acme %q", rt.TenantID, acme.ID)
	}

	// An unknown token cannot enroll anything.
	if _, err := svc.EnrollRunner(ctx, "rt_unknown_token", accountID, "ghost", "Ghost"); !errors.Is(err, ErrInvalidRegistrationToken) {
		t.Errorf("EnrollRunner(unknown token) error = %v, want ErrInvalidRegistrationToken", err)
	}
}
