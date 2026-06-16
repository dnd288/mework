package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInstructionsTooLarge = errors.New("instructions exceed 64KB limit")
)

type EnqueueParams struct {
	AccountID           string
	RuntimeID           string
	ExternalTaskID      string
	ExternalEventID     string
	ProviderCode        string
	ExternalActorID     string
	TaskTitle           string
	TaskDescription     string
	ProfileBodySnapshot *string
	Workflow            string
	Instructions        string
	TTLDuration         time.Duration
}

// Enqueue inserts a new job into the queue. It handles duplicate event IDs gracefully by returning empty ID and nil error.
func Enqueue(ctx context.Context, pool *pgxpool.Pool, p EnqueueParams) (string, error) {
	if p.AccountID == "" || p.RuntimeID == "" || p.ExternalTaskID == "" || p.Instructions == "" {
		return "", errors.New("missing required job fields")
	}

	if len(p.Instructions) > 65536 {
		return "", ErrInstructionsTooLarge
	}

	ttl := p.TTLDuration
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	ttlExpiresAt := time.Now().Add(ttl)

	var jobID string
	err := pool.QueryRow(ctx, `
		INSERT INTO jobs (
			account_id, runtime_id, external_task_id, external_event_id, provider_code,
			external_actor_id, task_title, task_description, profile_body_snapshot,
			workflow, instructions, status, ttl_expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'queued', $12)
		RETURNING id
	`, p.AccountID, p.RuntimeID, p.ExternalTaskID, p.ExternalEventID, p.ProviderCode,
		p.ExternalActorID, p.TaskTitle, p.TaskDescription, p.ProfileBodySnapshot,
		p.Workflow, p.Instructions, ttlExpiresAt).Scan(&jobID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // duplicate key
			return "", nil // Graceful no-op
		}
		return "", err
	}

	return jobID, nil
}
