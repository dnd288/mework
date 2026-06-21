package orchestrator

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Sweeper struct {
	pool          *pgxpool.Pool
	interval      time.Duration
	writeBackFunc func(ctx context.Context, jobID string) error
}

func NewSweeper(pool *pgxpool.Pool, interval time.Duration, writeBackFunc func(ctx context.Context, jobID string) error) *Sweeper {
	if interval == 0 {
		interval = 10 * time.Second
	}
	return &Sweeper{
		pool:          pool,
		interval:      interval,
		writeBackFunc: writeBackFunc,
	}
}

// Start runs the sweeper goroutines in the background.
func (s *Sweeper) Start(ctx context.Context) {
	go s.runLoop(ctx)
}

func (s *Sweeper) runLoop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep runs all three sweeper functions once.
func (s *Sweeper) Sweep(ctx context.Context) {
	if err := s.SweepTTL(ctx); err != nil {
		log.Printf("Sweeper error in SweepTTL: %v", err)
	}
	if err := s.SweepClaimLeases(ctx); err != nil {
		log.Printf("Sweeper error in SweepClaimLeases: %v", err)
	}
	if err := s.SweepWriteBack(ctx); err != nil {
		log.Printf("Sweeper error in SweepWriteBack: %v", err)
	}
}

// SweepTTL transitions jobs with expired TTL to 'failed'.
func (s *Sweeper) SweepTTL(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'failed', last_error = 'job TTL expired', finished_at = NOW()
		WHERE status NOT IN ('done', 'failed') AND ttl_expires_at <= NOW()
	`)
	return err
}

// SweepClaimLeases transitions expired 'claimed' or 'running' leases back to 'queued'.
func (s *Sweeper) SweepClaimLeases(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'queued', claim_lease_until = NULL, attempts = attempts + 1
		WHERE status IN ('claimed', 'running') AND claim_lease_until <= NOW()
	`)
	return err
}

// SweepWriteBack retries write-back for completed jobs.
func (s *Sweeper) SweepWriteBack(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT id, finished_at, writeback_attempts
		FROM jobs
		WHERE writeback_status = 'pending' AND status IN ('done', 'failed') AND finished_at IS NOT NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type jobToRetry struct {
		id       string
		attempts int
	}

	var retries []jobToRetry
	now := time.Now()

	for rows.Next() {
		var id string
		var finishedAt time.Time
		var attempts int
		if err := rows.Scan(&id, &finishedAt, &attempts); err != nil {
			return err
		}

		if now.After(NextWritebackTime(finishedAt, attempts)) {
			retries = append(retries, jobToRetry{id: id, attempts: attempts})
		}
	}

	for _, r := range retries {
		s.retryWriteBack(ctx, r.id, r.attempts)
	}

	return nil
}

func (s *Sweeper) retryWriteBack(ctx context.Context, jobID string, attempts int) {
	if attempts >= 5 {
		// Safety check in case it's already capped
		_, err := s.pool.Exec(ctx, `
			UPDATE jobs
			SET writeback_status = 'failed', writeback_last_error = 'max retry attempts reached'
			WHERE id = $1
		`, jobID)
		if err != nil {
			log.Printf("Failed to set writeback failed for job %s: %v", jobID, err)
		}
		return
	}

	// Try write-back
	err := s.writeBackFunc(ctx, jobID)
	if err != nil {
		newAttempts := attempts + 1
		var query string
		var args []any
		if newAttempts >= 5 {
			query = `
				UPDATE jobs
				SET writeback_attempts = $1, writeback_last_error = $2, writeback_status = 'failed'
				WHERE id = $3
			`
			args = []any{newAttempts, err.Error() + " (max retry attempts reached)", jobID}
		} else {
			query = `
				UPDATE jobs
				SET writeback_attempts = $1, writeback_last_error = $2
				WHERE id = $3
			`
			args = []any{newAttempts, err.Error(), jobID}
		}

		_, dbErr := s.pool.Exec(ctx, query, args...)
		if dbErr != nil {
			log.Printf("Failed to update writeback attempts for job %s: %v", jobID, dbErr)
		}
	} else {
		// Success
		_, dbErr := s.pool.Exec(ctx, `
			UPDATE jobs
			SET writeback_status = 'success'
			WHERE id = $1
		`, jobID)
		if dbErr != nil {
			log.Printf("Failed to update writeback success for job %s: %v", jobID, dbErr)
		}
	}
}

// NextWritebackTime computes the next retry time using exponential backoff:
// 1st retry (attempts=1): 1m
// 2nd retry (attempts=2): 3m (1m + 2m)
// 3rd retry (attempts=3): 7m (3m + 4m)
// 4th retry (attempts=4): 15m (7m + 8m)
func NextWritebackTime(finishedAt time.Time, attempts int) time.Time {
	if attempts == 0 {
		return finishedAt
	}
	var delay time.Duration
	switch attempts {
	case 1:
		delay = 1 * time.Minute
	case 2:
		delay = 3 * time.Minute
	case 3:
		delay = 7 * time.Minute
	case 4:
		delay = 15 * time.Minute
	default:
		delay = 31 * time.Minute
	}
	return finishedAt.Add(delay)
}
