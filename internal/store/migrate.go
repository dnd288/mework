package store

import (
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// RunMigrations runs all pending migrations in the embedded FS against the given DSN.
func RunMigrations(dsn string) error {
	db, err := OpenSQLDB(dsn)
	if err != nil {
		return fmt.Errorf("failed to open DB for migrations: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Disable goose's verbose logging to keep tests and daemon logs clean,
	// but keeping errors visible.
	goose.SetLogger(goose.NopLogger())

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations up: %w", err)
	}

	return nil
}

// RollbackMigrations rolls back all migrations (Down) against the given DSN.
func RollbackMigrations(dsn string) error {
	db, err := OpenSQLDB(dsn)
	if err != nil {
		return fmt.Errorf("failed to open DB for rollback: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	goose.SetLogger(goose.NopLogger())

	if err := goose.DownTo(db, "migrations", 0); err != nil {
		return fmt.Errorf("failed to run migrations down: %w", err)
	}

	return nil
}
