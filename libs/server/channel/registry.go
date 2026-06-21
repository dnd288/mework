// Package channel provides a durable channel-to-session binding registry backed by
// Postgres with an in-memory cache. A "channel" is a (provider, resource) pair that
// maps to an active worker session.
package channel

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrAlreadyBound is returned when a bind is attempted for an already-bound channel key.
	ErrAlreadyBound = errors.New("channel key already bound")
)

// Registry is the interface for managing channel-to-session bindings.
type Registry interface {
	// Bind creates a new channel session binding. Returns ErrAlreadyBound if the
	// channel key is already bound.
	Bind(ctx context.Context, channelKey, sessionID, runnerID, provider, resourceID, spec string) error
	// Unbind removes a channel binding, setting the row status to 'closed'.
	Unbind(ctx context.Context, channelKey string) error
	// Lookup returns the session ID for the given channel key, or empty string
	// if not found. Checks the in-memory cache first, then falls back to DB.
	Lookup(ctx context.Context, channelKey string) (string, error)
	// RunnerActiveChannelCount returns the number of active channel bindings for
	// the given runner.
	RunnerActiveChannelCount(ctx context.Context, runnerID string) (int, error)
	// PopulateCache loads all active channel sessions from the DB into the
	// in-memory cache. Called on server startup.
	PopulateCache(ctx context.Context) error
	// Status returns the current lifecycle status of a channel (active, draining, closed).
	// Returns empty string if the channel key is not found.
	Status(ctx context.Context, channelKey string) (string, error)
	// SetStatus updates the lifecycle status of a channel.
	SetStatus(ctx context.Context, channelKey, status string) error
}

// PostgresRegistry implements Registry with a Postgres backend and sync.Map cache.
type PostgresRegistry struct {
	pool  *pgxpool.Pool
	cache *sync.Map
}

// NewPostgresRegistry creates a new PostgresRegistry.
func NewPostgresRegistry(pool *pgxpool.Pool) Registry {
	return &PostgresRegistry{
		pool:  pool,
		cache: &sync.Map{},
	}
}

// Bind creates a new channel session binding. Uses an advisory lock on the
// hashed channel key to prevent concurrent double-provisioning.
func (r *PostgresRegistry) Bind(ctx context.Context, channelKey, sessionID, runnerID, provider, resourceID, spec string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Advisory xact lock prevents concurrent bind for the same key
	hash := hashChannelKey(channelKey)
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", hash)
	if err != nil {
		return err
	}

	// Check if already bound
	var existing string
	err = tx.QueryRow(ctx, "SELECT channel_key FROM channel_sessions WHERE channel_key = $1", channelKey).Scan(&existing)
	if err == nil {
		return ErrAlreadyBound
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO channel_sessions (channel_key, session_id, provider_code, resource_id, runner_id, spec, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')
	`, channelKey, sessionID, provider, resourceID, runnerID, spec)
	if err != nil {
		return err
	}

	// Update cache
	r.cache.Store(channelKey, sessionID)

	return tx.Commit(ctx)
}

// Unbind sets the channel session status to 'closed' and removes the entry from cache.
func (r *PostgresRegistry) Unbind(ctx context.Context, channelKey string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE channel_sessions SET status = 'closed', closed_at = NOW() WHERE channel_key = $1
	`, channelKey)
	if err != nil {
		return err
	}
	r.cache.Delete(channelKey)
	return nil
}

// Lookup returns the session ID for the given channel key. Checks the in-memory
// cache first; on miss, queries the DB and populates the cache.
func (r *PostgresRegistry) Lookup(ctx context.Context, channelKey string) (string, error) {
	// Cache hit
	if val, ok := r.cache.Load(channelKey); ok {
		return val.(string), nil
	}

	// Cache miss — query DB
	var sessionID string
	err := r.pool.QueryRow(ctx, `
		SELECT session_id FROM channel_sessions WHERE channel_key = $1 AND status = 'active'
	`, channelKey).Scan(&sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	// Populate cache
	r.cache.Store(channelKey, sessionID)
	return sessionID, nil
}

// RunnerActiveChannelCount returns the number of active channel bindings for a runner.
func (r *PostgresRegistry) RunnerActiveChannelCount(ctx context.Context, runnerID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM channel_sessions WHERE runner_id = $1 AND status = 'active'
	`, runnerID).Scan(&count)
	return count, err
}

// PopulateCache loads all active channel sessions from the DB into the in-memory cache.
func (r *PostgresRegistry) PopulateCache(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT channel_key, session_id FROM channel_sessions WHERE status = 'active'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var key, sessionID string
		if err := rows.Scan(&key, &sessionID); err != nil {
			return err
		}
		r.cache.Store(key, sessionID)
	}
	return nil
}

// Status returns the current lifecycle status of a channel. Returns empty
// string if the channel key is not found.
func (r *PostgresRegistry) Status(ctx context.Context, channelKey string) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT status FROM channel_sessions WHERE channel_key = $1
	`, channelKey).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return status, nil
}

// SetStatus updates the lifecycle status of a channel.
func (r *PostgresRegistry) SetStatus(ctx context.Context, channelKey, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE channel_sessions SET status = $1 WHERE channel_key = $2
	`, status, channelKey)
	return err
}

// hashChannelKey produces a 64-bit hash for advisory locking.
func hashChannelKey(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}
