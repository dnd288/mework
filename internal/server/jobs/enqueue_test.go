package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/internal/store"
)

func TestEnqueue(t *testing.T) {
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

	_, err = pool.Exec(ctx, "DELETE FROM jobs; DELETE FROM runtimes; DELETE FROM accounts;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	// Setup account & runtime
	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	var runtimeID string
	err = pool.QueryRow(ctx, "INSERT INTO runtimes (account_id, code, label, token_lookup) VALUES ($1, 'dev', 'Dev Machine', 'lookup-hash') RETURNING id", accountID).Scan(&runtimeID)
	if err != nil {
		t.Fatalf("failed to insert runtime: %v", err)
	}

	profileSnap := "snapshotted profile prompt content"
	params := EnqueueParams{
		AccountID:           accountID,
		RuntimeID:           runtimeID,
		ExternalTaskID:      "task-1",
		ExternalEventID:     "event-1",
		ProviderCode:        "mello",
		ExternalActorID:     "user-1",
		TaskTitle:           "Title",
		TaskDescription:     "Desc",
		ProfileBodySnapshot: &profileSnap,
		Instructions:        "some instructions",
	}

	// 1. Success case
	jobID, err := Enqueue(ctx, pool, params)
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}
	if jobID == "" {
		t.Error("expected non-empty job ID")
	}

	// Verify snapshots and values in DB
	var dbTitle, dbDesc, dbInst string
	var dbSnap *string
	err = pool.QueryRow(ctx, `
		SELECT task_title, task_description, instructions, profile_body_snapshot
		FROM jobs WHERE id = $1
	`, jobID).Scan(&dbTitle, &dbDesc, &dbInst, &dbSnap)
	if err != nil {
		t.Fatalf("failed to query enqueued job: %v", err)
	}

	if dbTitle != "Title" || dbDesc != "Desc" || dbInst != "some instructions" {
		t.Errorf("unexpected database values: title=%s, desc=%s, inst=%s", dbTitle, dbDesc, dbInst)
	}
	if dbSnap == nil || *dbSnap != profileSnap {
		t.Errorf("unexpected profile snapshot in database: %v", dbSnap)
	}

	// 2. Duplicate key (graceful no-op)
	dupJobID, err := Enqueue(ctx, pool, params)
	if err != nil {
		t.Fatalf("expected duplicate to be handled gracefully without error, got: %v", err)
	}
	if dupJobID != "" {
		t.Errorf("expected duplicate job ID to be empty, got: %s", dupJobID)
	}
}
