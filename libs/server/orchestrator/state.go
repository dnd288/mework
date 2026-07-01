package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidTransition = errors.New("invalid job status transition")
	ErrJobNotFound       = errors.New("job not found")
)

// TransitionJobState moves a job to a new status, enforcing the state machine rules in a transaction with row locking.
func TransitionJobState(ctx context.Context, pool *pgxpool.Pool, jobID, targetStatus string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Fetch current status with row lock
	var currentStatus string
	err = tx.QueryRow(ctx, "SELECT status FROM jobs WHERE id = $1 FOR UPDATE", jobID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobNotFound
		}
		return err
	}

	// 2. Enforce rules
	if currentStatus == targetStatus {
		// Idempotent terminal or non-terminal transition is a no-op
		return nil
	}

	// Terminal states cannot transition to other states
	if currentStatus == "done" || currentStatus == "failed" {
		return fmt.Errorf("%w: cannot transition from terminal state %q to %q", ErrInvalidTransition, currentStatus, targetStatus)
	}

	valid := false
	switch currentStatus {
	case "queued":
		valid = (targetStatus == "claimed" || targetStatus == "failed")
	case "claimed":
		valid = (targetStatus == "running" || targetStatus == "done" || targetStatus == "failed" || targetStatus == "queued")
	case "running":
		valid = (targetStatus == "done" || targetStatus == "failed" || targetStatus == "queued")
	}

	if !valid {
		return fmt.Errorf("%w: from %q to %q", ErrInvalidTransition, currentStatus, targetStatus)
	}

	// 3. Perform update with automatic started_at / finished_at setting
	var updateQuery string
	switch targetStatus {
	case "running":
		updateQuery = "UPDATE jobs SET status = $1, started_at = NOW() WHERE id = $2"
	case "done", "failed":
		updateQuery = "UPDATE jobs SET status = $1, finished_at = NOW() WHERE id = $2"
	default:
		updateQuery = "UPDATE jobs SET status = $1 WHERE id = $2"
	}

	_, err = tx.Exec(ctx, updateQuery, targetStatus, jobID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
