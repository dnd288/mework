package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthHandler_NilPool(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	handler := HealthHandler(nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHealthHandler_Integration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration health test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	// 1. Test when DB is up (200 OK)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	handler := HealthHandler(pool)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d. body: %s", w.Code, w.Body.String())
	}

	// 2. Test when DB pool is closed (503 Service Unavailable)
	pool.Close()

	// Wait briefly for pool to fully mark connections closed
	time.Sleep(100 * time.Millisecond)

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)

	if w2.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d. body: %s", w2.Code, w2.Body.String())
	}
}
