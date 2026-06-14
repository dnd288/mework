package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestMigrations(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration migration test")
	}

	ctx := context.Background()

	// 1. Force a complete rollback first to ensure we start from a clean state
	err := RollbackMigrations(dsn)
	if err != nil {
		t.Fatalf("failed to rollback migrations on startup: %v", err)
	}

	// Connect with pgx to inspect schema
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer conn.Close(ctx)

	// Verify no tables exist
	assertTableCount(t, ctx, conn, 0)

	// 2. Run migrations Up
	err = RunMigrations(dsn)
	if err != nil {
		t.Fatalf("failed to run migrations up: %v", err)
	}

	// 3. Verify tables exist (including goose_db_version)
	assertTableExists(t, ctx, conn, "accounts")
	assertTableExists(t, ctx, conn, "account_boards")
	assertTableExists(t, ctx, conn, "runtimes")
	assertTableExists(t, ctx, conn, "profiles")
	assertTableExists(t, ctx, conn, "jobs")

	// Verify columns on account_boards
	assertColumnExists(t, ctx, conn, "account_boards", "account_id")
	assertColumnExists(t, ctx, conn, "account_boards", "mello_board_id")

	// Verify schema additions on runtimes
	assertColumnExists(t, ctx, conn, "runtimes", "token_lookup")

	// Verify schema additions on jobs
	assertColumnExists(t, ctx, conn, "jobs", "attempts")
	assertColumnExists(t, ctx, conn, "jobs", "last_error")
	assertColumnExists(t, ctx, conn, "jobs", "ticket_title")
	assertColumnExists(t, ctx, conn, "jobs", "ticket_description")
	assertColumnExists(t, ctx, conn, "jobs", "profile_body_snapshot")

	// Verify indexes exist
	assertIndexExists(t, ctx, conn, "idx_jobs_claim")
	assertIndexExists(t, ctx, conn, "idx_jobs_one_active_per_runtime")

	// 4. Rollback Down
	err = RollbackMigrations(dsn)
	if err != nil {
		t.Fatalf("failed to rollback migrations: %v", err)
	}

	// Verify tables are removed (except potentially goose_db_version which might stay empty or be deleted depending on goose version)
	assertTableCount(t, ctx, conn, 0)
}

func assertTableExists(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName string) {
	t.Helper()
	var exists bool
	query := `SELECT EXISTS (
		SELECT FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name = $1
	)`
	err := conn.QueryRow(ctx, query, tableName).Scan(&exists)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !exists {
		t.Errorf("expected table %s to exist, but it does not", tableName)
	}
}

func assertColumnExists(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName, columnName string) {
	t.Helper()
	var exists bool
	query := `SELECT EXISTS (
		SELECT FROM information_schema.columns
		WHERE table_schema = 'public'
		AND table_name = $1
		AND column_name = $2
	)`
	err := conn.QueryRow(ctx, query, tableName, columnName).Scan(&exists)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !exists {
		t.Errorf("expected column %s on table %s to exist, but it does not", columnName, tableName)
	}
}

func assertIndexExists(t *testing.T, ctx context.Context, conn *pgx.Conn, indexName string) {
	t.Helper()
	var exists bool
	query := `SELECT EXISTS (
		SELECT FROM pg_indexes
		WHERE schemaname = 'public'
		AND indexname = $1
	)`
	err := conn.QueryRow(ctx, query, indexName).Scan(&exists)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if !exists {
		t.Errorf("expected index %s to exist, but it does not", indexName)
	}
}

func assertTableCount(t *testing.T, ctx context.Context, conn *pgx.Conn, expected int) {
	t.Helper()
	var count int
	query := `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name NOT LIKE 'pg_%'
		AND table_name NOT LIKE 'sql_%'`
	err := conn.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	// goose_db_version table may or may not exist after down migrations. We allow it to be 0 or 1 if it's goose_db_version.
	if expected == 0 {
		if count > 1 {
			t.Errorf("expected 0 user tables, got %d", count)
		}
	} else {
		if count != expected {
			t.Errorf("expected %d user tables, got %d", expected, count)
		}
	}
}
