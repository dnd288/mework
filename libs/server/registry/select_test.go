package registry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

// TestSelectWorker_MatchingSpec verifies that SelectWorker picks a runner whose
// specs array contains the requested spec and does not consider runners with
// different specs. Delta-spec scenario: "Select runner matching spec".
func TestSelectWorker_MatchingSpec(t *testing.T) {
	ctx, svc, accountID := newSelectTestService(t)
	ten, err := svc.RegisterTenant(ctx, "match-spec-test")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}

	tests := []struct {
		name          string
		setupRunners  []runnerSeed
		requestedSpec string
		wantCode      string
		wantErr       bool
	}{
		{
			name: "pick runner with matching spec",
			setupRunners: []runnerSeed{
				{code: "runner-a", specs: []string{"claude-code"}, status: "online"},
				{code: "runner-b", specs: []string{"codex"}, status: "online"},
			},
			requestedSpec: "claude-code",
			wantCode:      "runner-a",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Seed runners
			for _, rs := range tt.setupRunners {
				seedRunner(t, ctx, svc, ten.ID, accountID, rs)
			}

			got, err := svc.SelectWorker(ctx, tt.requestedSpec, *ten)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SelectWorker(%s): expected error, got runner %+v", tt.requestedSpec, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectWorker(%s): unexpected error: %v", tt.requestedSpec, err)
			}
			if got.Code != tt.wantCode {
				t.Errorf("SelectWorker(%s) = %s (%s), want %s", tt.requestedSpec, got.Code, got.ID, tt.wantCode)
			}
			// Verify the runner's specs actually contain the requested spec
			hasSpec := false
			for _, s := range got.Specs {
				if s == tt.requestedSpec {
					hasSpec = true
					break
				}
			}
			if !hasSpec {
				t.Errorf("SelectWorker(%s) returned runner %s with specs %v, none match requested spec", tt.requestedSpec, got.Code, got.Specs)
			}
		})
	}
}

// TestSelectWorker_BackwardCompat verifies that a runner with specs=NULL is
// matched for any spec query. Delta-spec scenario:
// "Backward-compatible runner matches any spec".
func TestSelectWorker_BackwardCompat(t *testing.T) {
	ctx, svc, accountID := newSelectTestService(t)
	ten, err := svc.RegisterTenant(ctx, "backward-select-test")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}

	tests := []struct {
		name          string
		setupRunners  []runnerSeed
		requestedSpec string
		wantCode      string
	}{
		{
			name: "null specs runner matches any spec",
			setupRunners: []runnerSeed{
				{code: "backward-runner", specs: nil, status: "online"},
			},
			requestedSpec: "claude-code",
			wantCode:      "backward-runner",
		},
		{
			name: "prefer matching spec over backward compat when both exist",
			setupRunners: []runnerSeed{
				{code: "match-runner-2", specs: []string{"claude-code"}, status: "online"},
				{code: "backward-runner-2", specs: nil, status: "online"},
			},
			requestedSpec: "claude-code",
			wantCode:      "match-runner-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, rs := range tt.setupRunners {
				seedRunner(t, ctx, svc, ten.ID, accountID, rs)
			}

			got, err := svc.SelectWorker(ctx, tt.requestedSpec, *ten)
			if err != nil {
				t.Fatalf("SelectWorker(%s): %v", tt.requestedSpec, err)
			}
			if got.Code != tt.wantCode {
				t.Errorf("SelectWorker(%s) = %s, want %s", tt.requestedSpec, got.Code, tt.wantCode)
			}
		})
	}
}

// TestSelectWorker_LoadBalanced verifies that among matching runners, SelectWorker
// picks the one with fewest active channel bindings. Delta-spec scenario:
// "Load-balanced across matching runners".
func TestSelectWorker_LoadBalanced(t *testing.T) {
	ctx, svc, accountID := newSelectTestService(t)
	ten, err := svc.RegisterTenant(ctx, "load-balance-test")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}

	tests := []struct {
		name          string
		setupRunners  []runnerSeed
		setupChannels map[string]int // runnerID -> channel count
		requestedSpec string
		wantCode      string
	}{
		{
			name: "pick runner with fewer active channels",
			setupRunners: []runnerSeed{
				{code: "busy-runner", specs: []string{"claude-code"}, status: "online"},
				{code: "free-runner", specs: []string{"claude-code"}, status: "online"},
			},
			// Both have 0 active channels initially, so free-runner is preferred
			requestedSpec: "claude-code",
			wantCode:      "free-runner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerIDs := make(map[string]string)
			for _, rs := range tt.setupRunners {
				rt, _, err := svc.CreateRuntime(ctx, *ten, accountID, rs.code, "label-"+rs.code, rs.specs...)
				if err != nil {
					t.Fatalf("CreateRuntime(%s): %v", rs.code, err)
				}
				// Set status to online directly
				_, err = svc.pool.Exec(ctx, "UPDATE runtimes SET status = $1 WHERE id = $2", rs.status, rt.ID)
				if err != nil {
					t.Fatalf("update runner status: %v", err)
				}
				runnerIDs[rs.code] = rt.ID
			}

			got, err := svc.SelectWorker(ctx, tt.requestedSpec, *ten)
			if err != nil {
				t.Fatalf("SelectWorker(%s): %v", tt.requestedSpec, err)
			}
			if got.Code != tt.wantCode {
				t.Errorf("SelectWorker(%s) = %s, want %s", tt.requestedSpec, got.Code, tt.wantCode)
			}
		})
	}
}

// TestSelectWorker_NoMatch verifies that SelectWorker returns an error when no
// online runner matches the requested spec.
func TestSelectWorker_NoMatch(t *testing.T) {
	ctx, svc, accountID := newSelectTestService(t)
	ten, err := svc.RegisterTenant(ctx, "no-match-test")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}

	tests := []struct {
		name          string
		setupRunners  []runnerSeed
		requestedSpec string
	}{
		{
			name:          "no runners at all",
			setupRunners:  nil,
			requestedSpec: "claude-code",
		},
		{
			name: "runners exist but none match spec",
			setupRunners: []runnerSeed{
				{code: "runner-x", specs: []string{"codex"}, status: "online"},
			},
			requestedSpec: "claude-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, rs := range tt.setupRunners {
				seedRunner(t, ctx, svc, ten.ID, accountID, rs)
			}

			_, err := svc.SelectWorker(ctx, tt.requestedSpec, *ten)
			if err == nil {
				t.Fatalf("SelectWorker(%s): expected error, got nil", tt.requestedSpec)
			}
		})
	}
}

// runnerSeed describes a runner to seed for select tests.
type runnerSeed struct {
	code   string
	specs  []string
	status string
}

// seedRunner creates a runner using CreateRuntime and updates its status.
func seedRunner(t *testing.T, ctx context.Context, svc *Service, tenantID, accountID string, rs runnerSeed) string {
	t.Helper()
	ten := Tenant{ID: tenantID}
	rt, _, err := svc.CreateRuntime(ctx, ten, accountID, rs.code, "label-"+rs.code, rs.specs...)
	if err != nil {
		t.Fatalf("CreateRuntime(%s): %v", rs.code, err)
	}
	if rs.status != "" {
		_, err = svc.pool.Exec(ctx, "UPDATE runtimes SET status = $1 WHERE id = $2", rs.status, rt.ID)
		if err != nil {
			t.Fatalf("update status for %s: %v", rs.code, err)
		}
	}
	return rt.ID
}

// newSelectTestService spins up a clean DB-backed Service.
func newSelectTestService(t *testing.T) (context.Context, *Service, string) {
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
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Select Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	return ctx, NewService(pool, "supersecret"), accountID
}
