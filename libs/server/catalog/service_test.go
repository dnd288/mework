package catalog

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

func TestProfileService(t *testing.T) {
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
	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM profiles; DELETE FROM accounts;")
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

	svc := NewService(pool)

	// 1. Create a profile
	p1, err := svc.CreateProfile(ctx, accountID1, CreateProfileRequest{
		Name:        "dev",
		Body:        "my system prompt",
		BackendHint: "claude",
		Harness:     "ck",
		WorkflowConfig: map[string]any{
			"version": float64(1),
		},
	})

	if err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	if p1.Name != "dev" || p1.Body != "my system prompt" {
		t.Errorf("unexpected profile: %+v", p1)
	}

	// 2. Duplicate profile check (should return ErrDuplicateName)
	_, err = svc.CreateProfile(ctx, accountID1, CreateProfileRequest{
		Name: "dev",
		Body: "another prompt",
	})
	if err != ErrDuplicateName {
		t.Errorf("expected ErrDuplicateName, got: %v", err)
	}

	// Allow duplicate name across accounts
	p2, err := svc.CreateProfile(ctx, accountID2, CreateProfileRequest{
		Name: "dev",
		Body: "account 2 prompt",
	})
	if err != nil {
		t.Fatalf("failed to create profile for account 2: %v", err)
	}
	if p2.AccountID != accountID2 {
		t.Errorf("expected account ID %s, got %s", accountID2, p2.AccountID)
	}

	// 3. Body size limit check (>64KB)
	largeBody := strings.Repeat("A", 65537)
	_, err = svc.CreateProfile(ctx, accountID1, CreateProfileRequest{
		Name: "large",
		Body: largeBody,
	})
	if err != ErrBodyTooLarge {
		t.Errorf("expected ErrBodyTooLarge, got: %v", err)
	}

	// 4. Get and List profiles
	got, err := svc.GetProfile(ctx, accountID1, "dev")
	if err != nil {
		t.Fatalf("failed to get profile: %v", err)
	}
	if got.ID != p1.ID {
		t.Errorf("expected profile ID %s, got %s", p1.ID, got.ID)
	}

	list, err := svc.ListProfiles(ctx, accountID1)
	if err != nil {
		t.Fatalf("failed to list profiles: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 profile, got %d", len(list))
	}

	// 5. Update profile
	time.Sleep(1 * time.Second) // Ensure updated_at will change
	updated, err := svc.UpdateProfile(ctx, accountID1, "dev", UpdateProfileRequest{
		Body:        "updated system prompt",
		BackendHint: "opencode",
		Harness:     "custom",
		WorkflowConfig: map[string]any{
			"version": float64(2),
		},
	})
	if err != nil {
		t.Fatalf("failed to update profile: %v", err)
	}
	if updated.Body != "updated system prompt" || updated.BackendHint != "opencode" {
		t.Errorf("unexpected updated values: %+v", updated)
	}
	if updated.UpdatedAt == updated.CreatedAt {
		t.Error("expected UpdatedAt to be different from CreatedAt")
	}

	// 6. Cross-account check (IDOR checks)
	// Create another profile under accountID1 that does not exist under accountID2
	_, err = svc.CreateProfile(ctx, accountID1, CreateProfileRequest{
		Name: "only-on-1",
		Body: "secret body",
	})
	if err != nil {
		t.Fatalf("failed to create profile only-on-1: %v", err)
	}

	_, err = svc.GetProfile(ctx, accountID2, "only-on-1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account read, got: %v", err)
	}

	_, err = svc.UpdateProfile(ctx, accountID2, "only-on-1", UpdateProfileRequest{Body: "hack"})
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account update, got: %v", err)
	}

	// 7. Delete profile
	err = svc.DeleteProfile(ctx, accountID2, "only-on-1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account delete, got: %v", err)
	}

	err = svc.DeleteProfile(ctx, accountID1, "dev")
	if err != nil {
		t.Fatalf("failed to delete profile: %v", err)
	}

	_, err = svc.GetProfile(ctx, accountID1, "dev")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}
