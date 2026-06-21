package channel

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sweeper periodically closes orphaned channel bindings whose runner is offline
// or no longer exists. It runs on a configurable interval.
type Sweeper struct {
	pool     *pgxpool.Pool
	registry Registry
	interval time.Duration
}

// NewSweeper creates a new Sweeper with the given pool, registry, and interval.
func NewSweeper(pool *pgxpool.Pool, registry Registry, interval time.Duration) *Sweeper {
	return &Sweeper{
		pool:     pool,
		registry: registry,
		interval: interval,
	}
}

// Run performs a single sweep: finds all active channel bindings whose
// associated runner is offline or missing, and transitions them to closed.
func (s *Sweeper) Run(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT cs.channel_key
		FROM channel_sessions cs
		LEFT JOIN runtimes r ON cs.runner_id = r.id::text
		WHERE cs.status = 'active' AND (r.id IS NULL OR r.status != 'online')
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var channelKey string
		if err := rows.Scan(&channelKey); err != nil {
			return err
		}
		if err := s.registry.Unbind(ctx, channelKey); err != nil {
			slog.Warn("sweeper close failed", "channel_key", channelKey, "error", err)
			continue
		}
		slog.Info("sweeper closed orphaned channel", "channel_key", channelKey)
		count++
	}

	if count > 0 {
		slog.Info("sweeper completed", "closed_count", count)
	}
	return nil
}

// Start launches a background goroutine that runs the sweeper on a ticker.
// The sweeper stops when the context is cancelled.
func (s *Sweeper) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.Run(ctx); err != nil {
					slog.Warn("sweeper run failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
