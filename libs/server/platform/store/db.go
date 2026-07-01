package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store encapsulates the database pool and connection utilities.
type Store struct {
	Pool *pgxpool.Pool
}

// NewStore initializes a new PostgreSQL pgx pool.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	// Sane defaults for connection management
	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify we can reach the database
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{Pool: pool}, nil
}

// Close gracefully closes the pgx pool.
func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}

// OpenSQLDB returns a standard *sql.DB for compatibility (e.g. for migrations).
func OpenSQLDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sql database: %w", err)
	}

	// Verify connectivity
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sql database: %w", err)
	}

	return db, nil
}
