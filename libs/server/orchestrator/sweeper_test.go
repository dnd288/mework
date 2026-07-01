package orchestrator

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

func TestSweeperTTLAndLease(t *testing.T) {
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

	// 1. Insert an expired TTL job
	var expiredJobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (
			account_id, runtime_id, external_task_id, external_event_id, provider_code,
			task_title, task_description, instructions, status, ttl_expires_at
		) VALUES ($1, $2, 'task-1', 'event-1', 'mello', 'Title', 'Desc', 'inst', 'queued', NOW() - INTERVAL '1 second')
		RETURNING id
	`, accountID, runtimeID).Scan(&expiredJobID)
	if err != nil {
		t.Fatalf("failed to insert expired TTL job: %v", err)
	}

	// 2. Insert an expired claim lease job
	var expiredLeaseJobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (
			account_id, runtime_id, external_task_id, external_event_id, provider_code,
			task_title, task_description, instructions, status, ttl_expires_at, claim_lease_until, attempts
		) VALUES ($1, $2, 'task-2', 'event-2', 'mello', 'Title', 'Desc', 'inst', 'claimed', NOW() + INTERVAL '1 hour', NOW() - INTERVAL '1 second', 1)
		RETURNING id
	`, accountID, runtimeID).Scan(&expiredLeaseJobID)
	if err != nil {
		t.Fatalf("failed to insert expired lease job: %v", err)
	}

	// 3. Setup mock writeback function
	mockWriteBackCalled := make(map[string]int)
	var mu sync.Mutex
	mockWriteBack := func(ctx context.Context, jobID string) error {
		mu.Lock()
		defer mu.Unlock()
		mockWriteBackCalled[jobID]++
		if jobID == "fail-me" {
			return errors.New("writeback failure")
		}
		return nil
	}

	sweeper := NewSweeper(pool, 100*time.Millisecond, mockWriteBack)

	// Run sweep once
	sweeper.Sweep(ctx)

	// Verify TTL job is failed
	var ttlStatus, ttlError string
	err = pool.QueryRow(ctx, "SELECT status, last_error FROM jobs WHERE id = $1", expiredJobID).Scan(&ttlStatus, &ttlError)
	if err != nil {
		t.Fatalf("failed to query TTL job: %v", err)
	}
	if ttlStatus != "failed" || ttlError != "job TTL expired" {
		t.Errorf("expected TTL job to be failed with TTL expired error, got status=%q, error=%q", ttlStatus, ttlError)
	}

	// Verify lease job is returned to queued and attempts incremented
	var leaseStatus string
	var leaseAttempts int
	err = pool.QueryRow(ctx, "SELECT status, attempts FROM jobs WHERE id = $1", expiredLeaseJobID).Scan(&leaseStatus, &leaseAttempts)
	if err != nil {
		t.Fatalf("failed to query lease job: %v", err)
	}
	if leaseStatus != "queued" || leaseAttempts != 2 {
		t.Errorf("expected lease job to be queued with attempts=2, got status=%q, attempts=%d", leaseStatus, leaseAttempts)
	}
}

func TestSweeperWriteBackRetry(t *testing.T) {
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

	// 1. Success writeback job (finished, writeback pending, next retry past current time)
	var successJobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (
			account_id, runtime_id, external_task_id, external_event_id, provider_code,
			task_title, task_description, instructions, status, ttl_expires_at,
			writeback_status, finished_at, writeback_attempts
		) VALUES ($1, $2, 'task-3', 'event-3', 'mello', 'Title', 'Desc', 'inst', 'done', NOW() + INTERVAL '1 hour',
			'pending', NOW() - INTERVAL '10 seconds', 0)
		RETURNING id
	`, accountID, runtimeID).Scan(&successJobID)
	if err != nil {
		t.Fatalf("failed to insert success job: %v", err)
	}

	// 2. Failed writeback job (attempts=1, next retry in 1 minute, current time not reached -> should NOT be retried)
	var notYetJobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (
			account_id, runtime_id, external_task_id, external_event_id, provider_code,
			task_title, task_description, instructions, status, ttl_expires_at,
			writeback_status, finished_at, writeback_attempts
		) VALUES ($1, $2, 'task-4', 'event-4', 'mello', 'Title', 'Desc', 'inst', 'done', NOW() + INTERVAL '1 hour',
			'pending', NOW(), 1)
		RETURNING id
	`, accountID, runtimeID).Scan(&notYetJobID)
	if err != nil {
		t.Fatalf("failed to insert not-yet job: %v", err)
	}

	// 3. Writeback failure mock
	mockWriteBackCalled := make(map[string]int)
	var mu sync.Mutex
	mockWriteBack := func(ctx context.Context, jobID string) error {
		mu.Lock()
		defer mu.Unlock()
		mockWriteBackCalled[jobID]++
		if jobID == successJobID {
			return nil
		}
		return errors.New("failure")
	}

	sweeper := NewSweeper(pool, 100*time.Millisecond, mockWriteBack)

	// Run sweep once
	sweeper.Sweep(ctx)

	// Check success job is marked success in database
	var successStatus string
	err = pool.QueryRow(ctx, "SELECT writeback_status FROM jobs WHERE id = $1", successJobID).Scan(&successStatus)
	if err != nil {
		t.Fatalf("failed to query success job: %v", err)
	}
	if successStatus != "success" {
		t.Errorf("expected writeback_status to be success, got: %s", successStatus)
	}

	// Check mock writeback was called for success job
	mu.Lock()
	calls := mockWriteBackCalled[successJobID]
	mu.Unlock()
	if calls != 1 {
		t.Errorf("expected 1 writeback call for success job, got %d", calls)
	}

	// Check not-yet job remained pending
	var notYetStatus string
	err = pool.QueryRow(ctx, "SELECT writeback_status FROM jobs WHERE id = $1", notYetJobID).Scan(&notYetStatus)
	if err != nil {
		t.Fatalf("failed to query not-yet job: %v", err)
	}
	if notYetStatus != "pending" {
		t.Errorf("expected not-yet job to remain pending, got: %s", notYetStatus)
	}

	mu.Lock()
	notYetCalls := mockWriteBackCalled[notYetJobID]
	mu.Unlock()
	if notYetCalls != 0 {
		t.Errorf("expected 0 writeback calls for not-yet job, got %d", notYetCalls)
	}
}
