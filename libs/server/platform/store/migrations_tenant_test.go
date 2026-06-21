package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
)

// scopedTables are the tenant-scoped resource tables that migration 000002 must
// key by tenant: each gets a tenant_id column plus an index on it.
var scopedTables = []string{
	"provider_connections",
	"account_identities",
	"watched_containers",
	"runtimes",
	"profiles",
	"jobs",
}

// TestTenancyMigration realizes the tenancy delta-spec at the schema level
// ("Register an isolated tenant" / "Tenants are isolated from each other"):
//   - a tenants table exists,
//   - a default tenant row exists after migration,
//   - every tenant-scoped table has an indexed tenant_id column, and
//   - every pre-existing row backfills to the default tenant id.
//
// DB-backed; skips without TEST_DATABASE_URL.
func TestTenancyMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration tenancy migration test")
	}

	ctx := context.Background()

	// Start from a clean slate.
	if err := RollbackMigrations(dsn); err != nil {
		t.Fatalf("failed to rollback migrations on startup: %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer conn.Close(ctx)

	assertTableCount(t, ctx, conn, 0)

	// Apply migration 000001 only, then seed a pre-existing row so we can prove the
	// 000002 backfill assigns it to the default tenant.
	if err := migrateUpTo(dsn, 1); err != nil {
		t.Fatalf("failed to migrate to 000001: %v", err)
	}

	var accountID string
	if err := conn.QueryRow(ctx,
		"INSERT INTO accounts (name) VALUES ('Pre-existing') RETURNING id",
	).Scan(&accountID); err != nil {
		t.Fatalf("failed to seed account: %v", err)
	}

	var preRuntimeID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO runtimes (account_id, code, label, token_lookup)
		VALUES ($1, 'pre_code', 'Pre Label', 'pre_lookup')
		RETURNING id
	`, accountID).Scan(&preRuntimeID); err != nil {
		t.Fatalf("failed to seed pre-existing runtime: %v", err)
	}

	// Now apply the tenancy migration (000002) on top.
	if err := RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations up: %v", err)
	}

	t.Cleanup(func() {
		_ = RollbackMigrations(dsn)
	})

	// (a) tenants table exists.
	assertTableExists(t, ctx, conn, "tenants")

	// (b) a default tenant row exists after migration; capture its id.
	var defaultTenantID string
	var tenantCount int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM tenants").Scan(&tenantCount); err != nil {
		t.Fatalf("failed to count tenants: %v", err)
	}
	if tenantCount < 1 {
		t.Fatalf("expected at least one (default) tenant row after migration, got %d", tenantCount)
	}
	if err := conn.QueryRow(ctx, "SELECT id FROM tenants ORDER BY created_at ASC LIMIT 1").Scan(&defaultTenantID); err != nil {
		t.Fatalf("failed to read default tenant id: %v", err)
	}
	if defaultTenantID == "" {
		t.Fatal("expected a non-empty default tenant id")
	}

	// (c) every tenant-scoped table has a tenant_id column with a NOT NULL constraint
	// and an index covering it.
	for _, table := range scopedTables {
		assertColumnExists(t, ctx, conn, table, "tenant_id")
		assertColumnNotNull(t, ctx, conn, table, "tenant_id")
		assertIndexedColumn(t, ctx, conn, table, "tenant_id")
	}

	// (d) every pre-existing row backfills to the default tenant id.
	var gotTenantID string
	if err := conn.QueryRow(ctx,
		"SELECT tenant_id FROM runtimes WHERE id = $1", preRuntimeID,
	).Scan(&gotTenantID); err != nil {
		t.Fatalf("failed to read backfilled runtime tenant_id: %v", err)
	}
	if gotTenantID != defaultTenantID {
		t.Errorf("pre-existing runtime tenant_id = %q, want default tenant id %q", gotTenantID, defaultTenantID)
	}

	// Down reverses cleanly back to an empty schema.
	if err := RollbackMigrations(dsn); err != nil {
		t.Fatalf("failed to rollback migrations: %v", err)
	}
	assertTableCount(t, ctx, conn, 0)
}

// migrateUpTo applies embedded migrations up to (and including) the given goose
// version, so the test can seed pre-existing rows before the tenancy migration runs.
func migrateUpTo(dsn string, version int64) error {
	db, err := OpenSQLDB(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetLogger(goose.NopLogger())
	return goose.UpTo(db, "migrations", version)
}

// assertColumnNotNull verifies the named column is declared NOT NULL.
func assertColumnNotNull(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName, columnName string) {
	t.Helper()
	var isNullable string
	query := `SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`
	if err := conn.QueryRow(ctx, query, tableName, columnName).Scan(&isNullable); err != nil {
		t.Fatalf("query failed checking nullability of %s.%s: %v", tableName, columnName, err)
	}
	if isNullable != "NO" {
		t.Errorf("expected column %s.%s to be NOT NULL, but is_nullable=%q", tableName, columnName, isNullable)
	}
}

// assertIndexedColumn verifies at least one index on the table covers the column.
func assertIndexedColumn(t *testing.T, ctx context.Context, conn *pgx.Conn, tableName, columnName string) {
	t.Helper()
	var exists bool
	query := `SELECT EXISTS (
		SELECT 1
		FROM pg_index ix
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		WHERE n.nspname = 'public'
		  AND t.relname = $1
		  AND a.attname = $2
	)`
	if err := conn.QueryRow(ctx, query, tableName, columnName).Scan(&exists); err != nil {
		t.Fatalf("query failed checking index on %s.%s: %v", tableName, columnName, err)
	}
	if !exists {
		t.Errorf("expected an index covering column %s.%s, but found none", tableName, columnName)
	}
}
