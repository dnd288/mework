package orchestrator

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

func TestStateTransitions(t *testing.T) {
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

	params := EnqueueParams{
		AccountID:       accountID,
		RuntimeID:       runtimeID,
		ExternalTaskID:  "task-1",
		ExternalEventID: "event-1",
		ProviderCode:    "mello",
		ExternalActorID: "user-1",
		TaskTitle:       "Title",
		TaskDescription: "Desc",
		Instructions:    "some instructions",
	}

	jobID, err := Enqueue(ctx, pool, params)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// 1. Legal transition: queued -> claimed
	err = TransitionJobState(ctx, pool, jobID, "claimed")
	if err != nil {
		t.Errorf("expected queued -> claimed to be legal, got error: %v", err)
	}

	// 2. Legal transition: claimed -> running
	err = TransitionJobState(ctx, pool, jobID, "running")
	if err != nil {
		t.Errorf("expected claimed -> running to be legal, got error: %v", err)
	}

	// 3. Legal transition: running -> done
	err = TransitionJobState(ctx, pool, jobID, "done")
	if err != nil {
		t.Errorf("expected running -> done to be legal, got error: %v", err)
	}

	// 4. Idempotent terminal transition: done -> done (no-op)
	err = TransitionJobState(ctx, pool, jobID, "done")
	if err != nil {
		t.Errorf("expected done -> done to be legal (no-op), got error: %v", err)
	}

	// 5. Illegal transition: done -> running (should return ErrInvalidTransition)
	err = TransitionJobState(ctx, pool, jobID, "running")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition for done -> running, got: %v", err)
	}
}
