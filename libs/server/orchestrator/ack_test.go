package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/middleware"
	"mework/libs/server/platform/store"
)

func TestAckJob(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// 1. Enqueue job
	jobID, err := Enqueue(ctx, pool, EnqueueParams{
		AccountID:       accountID,
		RuntimeID:       runtimeID,
		ExternalTaskID:  "task-1",
		ExternalEventID: "event-1",
		ProviderCode:    "mello",
		TaskTitle:       "Title",
		TaskDescription: "Desc",
		Instructions:    "inst",
	})
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// Claim it first to make transition valid
	err = TransitionJobState(ctx, pool, jobID, "claimed")
	if err != nil {
		t.Fatalf("failed to claim job: %v", err)
	}

	ackHandlers := NewAckHandlers(pool, "secret")

	// Set up router with middleware setting runtime ID context
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.RuntimeIDKey, runtimeID)
			ctx = context.WithValue(ctx, middleware.AccountIDKey, accountID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Post("/api/v1/jobs/{id}/ack", ackHandlers.AckJob)

	// Test case 1: Ack running -> success (204)
	{
		body, _ := json.Marshal(AckRequest{Status: "running"})
		req := httptest.NewRequest("POST", "/api/v1/jobs/"+jobID+"/ack", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("id", jobID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d, body: %s", rec.Code, rec.Body.String())
		}

		// Verify status is running in DB
		var status string
		err = pool.QueryRow(ctx, "SELECT status FROM jobs WHERE id = $1", jobID).Scan(&status)
		if err != nil {
			t.Fatalf("failed to query status: %v", err)
		}
		if status != "running" {
			t.Errorf("expected job status running, got: %s", status)
		}
	}

	// Test case 2: Ack done (terminal) -> success (204) and triggers writeback setting result_summary and finished_at
	{
		summary := "work summary result"
		body, _ := json.Marshal(AckRequest{Status: "done", ResultSummary: &summary})
		req := httptest.NewRequest("POST", "/api/v1/jobs/"+jobID+"/ack", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("id", jobID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d, body: %s", rec.Code, rec.Body.String())
		}

		// Verify status and results in DB
		var status, resultSummary, writebackStatus string
		var finishedAt *time.Time
		err = pool.QueryRow(ctx, "SELECT status, result_summary, writeback_status, finished_at FROM jobs WHERE id = $1", jobID).Scan(&status, &resultSummary, &writebackStatus, &finishedAt)
		if err != nil {
			t.Fatalf("failed to query terminal job: %v", err)
		}
		if status != "done" {
			t.Errorf("expected job status done, got: %s", status)
		}
		if resultSummary != summary {
			t.Errorf("expected result_summary %q, got %q", summary, resultSummary)
		}
		if writebackStatus != "pending" && writebackStatus != "failed" {
			t.Errorf("expected writeback_status to be pending or failed (due to mock server failure), got: %s", writebackStatus)
		}
		if finishedAt == nil {
			t.Error("expected finished_at to be populated")
		}
	}
}
