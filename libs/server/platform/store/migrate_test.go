package store

import (
	"context"
	"os"
	"strings"
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

	// 3. Verify tables exist
	assertTableExists(t, ctx, conn, "accounts")
	assertTableExists(t, ctx, conn, "provider_connections")
	assertTableExists(t, ctx, conn, "account_identities")
	assertTableExists(t, ctx, conn, "watched_containers")
	assertTableExists(t, ctx, conn, "runtimes")
	assertTableExists(t, ctx, conn, "profiles")
	assertTableExists(t, ctx, conn, "jobs")

	// Verify dropped tables and columns
	assertTableDoesNotExist(t, ctx, conn, "account_boards")
	assertColumnDoesNotExist(t, ctx, conn, "accounts", "mello_user_id")

	// Verify columns on provider_connections
	assertColumnExists(t, ctx, conn, "provider_connections", "id")
	assertColumnExists(t, ctx, conn, "provider_connections", "account_id")
	assertColumnExists(t, ctx, conn, "provider_connections", "provider_code")
	assertColumnExists(t, ctx, conn, "provider_connections", "webhook_secret")
	assertColumnExists(t, ctx, conn, "provider_connections", "mcp_url")
	assertColumnExists(t, ctx, conn, "provider_connections", "mcp_auth_enc")
	assertColumnExists(t, ctx, conn, "provider_connections", "config")
	assertColumnExists(t, ctx, conn, "provider_connections", "created_at")

	// Verify columns on account_identities
	assertColumnExists(t, ctx, conn, "account_identities", "account_id")
	assertColumnExists(t, ctx, conn, "account_identities", "provider_code")
	assertColumnExists(t, ctx, conn, "account_identities", "external_user_id")

	// Verify columns on watched_containers
	assertColumnExists(t, ctx, conn, "watched_containers", "account_id")
	assertColumnExists(t, ctx, conn, "watched_containers", "provider_code")
	assertColumnExists(t, ctx, conn, "watched_containers", "external_container_id")

	// Verify schema on runtimes
	assertColumnExists(t, ctx, conn, "runtimes", "token_lookup")

	// Verify schema on profiles
	assertColumnExists(t, ctx, conn, "profiles", "harness")
	assertColumnExists(t, ctx, conn, "profiles", "workflow_config")

	// Verify schema on jobs
	assertColumnExists(t, ctx, conn, "jobs", "external_task_id")
	assertColumnExists(t, ctx, conn, "jobs", "external_event_id")
	assertColumnExists(t, ctx, conn, "jobs", "provider_code")
	assertColumnExists(t, ctx, conn, "jobs", "external_actor_id")
	assertColumnExists(t, ctx, conn, "jobs", "writeback_status")
	assertColumnExists(t, ctx, conn, "jobs", "writeback_attempts")
	assertColumnExists(t, ctx, conn, "jobs", "writeback_last_error")
	assertColumnExists(t, ctx, conn, "jobs", "task_title")
	assertColumnExists(t, ctx, conn, "jobs", "task_description")

	// Verify obsolete jobs columns are removed
	assertColumnDoesNotExist(t, ctx, conn, "jobs", "mello_ticket_id")
	assertColumnDoesNotExist(t, ctx, conn, "jobs", "mello_comment_id")
	assertColumnDoesNotExist(t, ctx, conn, "jobs", "ticket_title")
	assertColumnDoesNotExist(t, ctx, conn, "jobs", "ticket_description")

	// Verify primary key constraints
	assertConstraintExists(t, ctx, conn, "account_identities", "p", []string{"account_id", "provider_code"})
	assertConstraintExists(t, ctx, conn, "watched_containers", "p", []string{"account_id", "provider_code", "external_container_id"})

	// Verify unique constraints
	assertConstraintExists(t, ctx, conn, "provider_connections", "u", []string{"account_id", "provider_code"})
	assertConstraintExists(t, ctx, conn, "account_identities", "u", []string{"provider_code", "external_user_id"})
	assertConstraintExists(t, ctx, conn, "watched_containers", "u", []string{"provider_code", "external_container_id"})
	assertConstraintExists(t, ctx, conn, "runtimes", "u", []string{"token_lookup"})
	assertConstraintExists(t, ctx, conn, "jobs", "u", []string{"provider_code", "external_event_id"})

	// Verify indexes exist
	assertIndexExists(t, ctx, conn, "idx_jobs_claim")
	assertIndexExists(t, ctx, conn, "idx_jobs_one_active_per_runtime")
	assertIndexExists(t, ctx, conn, "idx_jobs_writeback")
	assertIndexExists(t, ctx, conn, "idx_jobs_account_id")

	// Verify partial index definitions contain their filters
	assertIndexDefContains(t, ctx, conn, "idx_jobs_one_active_per_runtime", "status")
	assertIndexDefContains(t, ctx, conn, "idx_jobs_writeback", "writeback_status")
	assertIndexDefContains(t, ctx, conn, "idx_jobs_writeback", "pending")

	// ---- Channel Routing migration (000009) ----
	// Verify specs column on runtimes
	assertColumnExists(t, ctx, conn, "runtimes", "specs")

	// Verify channel_sessions table and columns
	assertTableExists(t, ctx, conn, "channel_sessions")
	assertColumnExists(t, ctx, conn, "channel_sessions", "channel_key")
	assertColumnExists(t, ctx, conn, "channel_sessions", "session_id")
	assertColumnExists(t, ctx, conn, "channel_sessions", "provider_code")
	assertColumnExists(t, ctx, conn, "channel_sessions", "resource_id")
	assertColumnExists(t, ctx, conn, "channel_sessions", "runner_id")
	assertColumnExists(t, ctx, conn, "channel_sessions", "spec")
	assertColumnExists(t, ctx, conn, "channel_sessions", "status")
	assertColumnExists(t, ctx, conn, "channel_sessions", "created_at")
	assertColumnExists(t, ctx, conn, "channel_sessions", "closed_at")

	// Verify indexes on channel_sessions
	assertIndexExists(t, ctx, conn, "idx_channel_sessions_runner_id")
	assertIndexExists(t, ctx, conn, "idx_channel_sessions_provider_resource")

	// Verify primary key constraint
	assertConstraintExists(t, ctx, conn, "channel_sessions", "p", []string{"channel_key"})

	// Verify status CHECK constraint allows active, draining, closed
	assertConstraintDefContains(t, ctx, conn, "channel_sessions", "channel_sessions_status_check", "active")
	assertConstraintDefContains(t, ctx, conn, "channel_sessions", "channel_sessions_status_check", "draining")
	assertConstraintDefContains(t, ctx, conn, "channel_sessions", "channel_sessions_status_check", "closed")

	// 4. Rollback Down
	err = RollbackMigrations(dsn)
	if err != nil {
		t.Fatalf("failed to rollback migrations: %v", err)
	}

	// Verify tables are removed
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

func assertTableDoesNotExist(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName string) {
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
	if exists {
		t.Errorf("expected table %s NOT to exist, but it does", tableName)
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

func assertColumnDoesNotExist(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName, columnName string) {
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
	if exists {
		t.Errorf("expected column %s on table %s NOT to exist, but it does", columnName, tableName)
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

func assertIndexDefContains(t *testing.T, ctx context.Context, conn *pgx.Conn, indexName, expectedPart string) {
	t.Helper()
	var indexDef string
	query := `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`
	err := conn.QueryRow(ctx, query, indexName).Scan(&indexDef)
	if err != nil {
		t.Fatalf("query failed fetching indexdef for %s: %v", indexName, err)
	}
	if !strings.Contains(indexDef, expectedPart) {
		t.Errorf("expected indexdef for %s to contain %q, but got %q", indexName, expectedPart, indexDef)
	}
}

func assertConstraintExists(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName, conType string, columns []string) {
	t.Helper()
	var exists bool
	query := `SELECT EXISTS (
		SELECT 1 FROM pg_constraint c
		JOIN pg_class t ON c.conrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		WHERE n.nspname = 'public'
		  AND t.relname = $1
		  AND c.contype = $2
		  AND (
			  SELECT array_agg(a.attname::text ORDER BY u.ord)
			  FROM unnest(c.conkey) WITH ORDINALITY u(attnum, ord)
			  JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
		  ) = $3::text[]
	)`
	err := conn.QueryRow(ctx, query, tableName, conType, columns).Scan(&exists)
	if err != nil {
		t.Fatalf("query failed checking constraint: %v", err)
	}
	if !exists {
		t.Errorf("expected constraint %s on table %s with columns %v to exist, but it does not", conType, tableName, columns)
	}
}

func assertConstraintDefContains(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName, constraintName, expectedPart string) {
	t.Helper()
	var def string
	query := `SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON c.conrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		WHERE n.nspname = 'public' AND t.relname = $1 AND c.conname = $2`
	err := conn.QueryRow(ctx, query, tableName, constraintName).Scan(&def)
	if err != nil {
		t.Fatalf("query failed fetching constraint def for %s on %s: %v", constraintName, tableName, err)
	}
	if !strings.Contains(def, expectedPart) {
		t.Errorf("expected constraint %s on %s to contain %q, but got %q", constraintName, tableName, expectedPart, def)
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
