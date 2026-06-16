package registry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/internal/store"
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
	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM accounts;")
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

	// 1. Create a runtime
	rt1, tok1, err := svc.CreateRuntime(ctx, accountID1, "rt_code_1", "Label 1")
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
	_, _, err = svc.CreateRuntime(ctx, accountID1, "rt_code_1", "Label 2")
	if err != ErrDuplicateCode {
		t.Errorf("expected ErrDuplicateCode, got: %v", err)
	}

	// 3. Create same code under a different account (should succeed)
	rt2, _, err := svc.CreateRuntime(ctx, accountID2, "rt_code_1", "Label 2")
	if err != nil {
		t.Fatalf("failed to create same code under different account: %v", err)
	}
	if rt2.AccountID != accountID2 {
		t.Errorf("expected account ID %s, got %s", accountID2, rt2.AccountID)
	}

	// 4. List runtimes
	rts, err := svc.ListRuntimes(ctx, accountID1)
	if err != nil {
		t.Fatalf("failed to list runtimes: %v", err)
	}

	if len(rts) != 1 {
		t.Errorf("expected 1 runtime, got %d", len(rts))
	}

	// 5. Delete runtime - IDOR check (delete rt1 with accountID2 should fail with ErrNotFound)
	err = svc.DeleteRuntime(ctx, accountID2, rt1.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account delete, got: %v", err)
	}

	// Delete with correct account should succeed
	err = svc.DeleteRuntime(ctx, accountID1, rt1.ID)
	if err != nil {
		t.Fatalf("failed to delete runtime: %v", err)
	}

	rts, err = svc.ListRuntimes(ctx, accountID1)
	if err != nil {
		t.Fatalf("failed to list runtimes: %v", err)
	}
	if len(rts) != 0 {
		t.Errorf("expected 0 runtimes, got %d", len(rts))
	}
}
