package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/internal/server/middleware"
	"mework/internal/store"
)

func TestClaimJob(t *testing.T) {
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

	// 1. Enqueue two jobs
	_, err = Enqueue(ctx, pool, EnqueueParams{
		AccountID:       accountID,
		RuntimeID:       runtimeID,
		ExternalTaskID:  "task-1",
		ExternalEventID: "event-1",
		ProviderCode:    "mello",
		TaskTitle:       "Title 1",
		TaskDescription: "Desc 1",
		Instructions:    "inst 1",
	})
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(100 * time.Millisecond) // Ensure created_at time separation

	_, err = Enqueue(ctx, pool, EnqueueParams{
		AccountID:       accountID,
		RuntimeID:       runtimeID,
		ExternalTaskID:  "task-2",
		ExternalEventID: "event-2",
		ProviderCode:    "mello",
		TaskTitle:       "Title 2",
		TaskDescription: "Desc 2",
		Instructions:    "inst 2",
	})
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	claimHandlers := NewClaimHandlers(pool)

	// Set up router with middleware setting the runtime ID context
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.RuntimeIDKey, runtimeID)
			ctx = context.WithValue(ctx, middleware.AccountIDKey, accountID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Post("/api/v1/jobs/claim", claimHandlers.ClaimJob)

	// Test case 1: First claim -> returns job 1 (oldest)
	var claimedJobID string
	{
		req := httptest.NewRequest("POST", "/api/v1/jobs/claim", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
		}

		var job Job
		if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
			t.Fatalf("failed to decode job response: %v", err)
		}

		if job.ExternalTaskID != "task-1" {
			t.Errorf("expected task-1 (oldest) to be claimed, got: %s", job.ExternalTaskID)
		}
		if job.Status != "claimed" {
			t.Errorf("expected job status to be claimed, got: %s", job.Status)
		}
		claimedJobID = job.ID
	}

	// Test case 2: Second claim while first is still active -> returns 204 (no-op limit)
	{
		req := httptest.NewRequest("POST", "/api/v1/jobs/claim", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204 No Content for runtime with active job, got %d", rec.Code)
		}
	}

	// Test case 3: Heartbeat on claimed job
	{
		ackHandlers := NewAckHandlers(pool, "secret", "http://localhost")
		r.Post("/api/v1/jobs/{id}/heartbeat", ackHandlers.Heartbeat)

		req := httptest.NewRequest("POST", "/api/v1/jobs/"+claimedJobID+"/heartbeat", nil)
		rec := httptest.NewRecorder()

		// Set up chi URL param manually
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("id", claimedJobID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204 No Content for heartbeat, got %d", rec.Code)
		}

		// Verify claim_lease_until was updated
		var claimLeaseUntil time.Time
		err = pool.QueryRow(ctx, "SELECT claim_lease_until FROM jobs WHERE id = $1", claimedJobID).Scan(&claimLeaseUntil)
		if err != nil {
			t.Fatalf("failed to query job: %v", err)
		}

		// It should be set to ~90 seconds in the future
		diff := time.Until(claimLeaseUntil)
		if diff < 80*time.Second || diff > 100*time.Second {
			t.Errorf("expected claim lease to be ~90 seconds in the future, got diff: %v", diff)
		}
	}
}
