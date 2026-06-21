// Package notify delivers outbound notifications on platform events such as
// run.done and run.failed. Notifications are sent to per-tenant webhook targets
// with HMAC signing, bounded retry on transient failure, and Postgres-backed
// delivery tracking so operators can observe delivery state.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/shared/core"
	"mework/shared/ports"
)

const (
	defaultMaxAttempts = 4
	initialBackoff     = 1 * time.Second
	maxBackoff         = 30 * time.Second
)

// Notifier sends outbound notifications and tracks delivery attempts in
// Postgres so retries are durable and operators can query delivery state.
type Notifier struct {
	pool   *pgxpool.Pool
	client *http.Client
}

// NewNotifier creates a new Notifier backed by the given Postgres pool.
func NewNotifier(pool *pgxpool.Pool) *Notifier {
	return &Notifier{
		pool: pool,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// compile-time check: *Notifier implements ports.Notifier
var _ ports.Notifier = (*Notifier)(nil)

// Notify sends an outbound notification for the given event. It looks up the
// tenant's configured webhook target, delivers the event with HMAC signing,
// and records the delivery attempt in Postgres.
func (n *Notifier) Notify(ctx context.Context, tenant core.TenantID, event core.NotifyEvent) error {
	if event.Kind == "" || event.RunID == "" {
		return fmt.Errorf("notify: kind and run_id are required")
	}

	// 1. Look up notification target for tenant.
	var targetURL, signingSecret string
	err := n.pool.QueryRow(ctx, `
		SELECT url, signing_secret FROM notification_targets
		WHERE tenant_id = $1 AND enabled = true
		LIMIT 1
	`, string(tenant)).Scan(&targetURL, &signingSecret)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no enabled notification target for tenant %s", tenant)
		}
		return fmt.Errorf("lookup notification target: %w", err)
	}

	// 2. Insert delivery record.
	var deliveryID string
	err = n.pool.QueryRow(ctx, `
		INSERT INTO notification_deliveries (tenant_id, run_id, event_kind, target_url, status, max_attempts, next_retry_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, NOW())
		RETURNING id
	`, string(tenant), event.RunID, event.Kind, targetURL, defaultMaxAttempts).Scan(&deliveryID)
	if err != nil {
		return fmt.Errorf("create delivery record: %w", err)
	}

	// 3. Deliver the webhook.
	statusCode, deliverErr := n.deliver(ctx, targetURL, signingSecret, event)

	// 4. Update delivery record based on result.
	if deliverErr != nil {
		n.recordFailure(ctx, deliveryID, 0, deliverErr.Error(), initialBackoff)
		return deliverErr
	}

	if statusCode >= 500 || statusCode == 429 {
		// Transient server error — mark for retry.
		errMsg := fmt.Sprintf("HTTP %d", statusCode)
		n.recordFailure(ctx, deliveryID, statusCode, errMsg, initialBackoff)
		return fmt.Errorf("webhook transient error: %s", errMsg)
	}

	if statusCode >= 400 {
		// Client error — terminal failure (the target rejected the event).
		errMsg := fmt.Sprintf("HTTP %d", statusCode)
		n.recordTerminalFailure(ctx, deliveryID, statusCode, errMsg)
		return fmt.Errorf("webhook rejected with %s", errMsg)
	}

	// Success (2xx).
	n.recordSuccess(ctx, deliveryID, statusCode)
	return nil
}

// DeliveryStatus returns the delivery history for a given run.
func (n *Notifier) DeliveryStatus(ctx context.Context, runID string) ([]ports.DeliveryResult, error) {
	rows, err := n.pool.Query(ctx, `
		SELECT id, run_id, event_kind, status, attempt_count,
		       COALESCE(last_status_code, 0), COALESCE(last_error, '')
		FROM notification_deliveries
		WHERE run_id = $1
		ORDER BY created_at DESC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("query delivery status: %w", err)
	}
	defer rows.Close()

	var results []ports.DeliveryResult
	for rows.Next() {
		var r ports.DeliveryResult
		if err := rows.Scan(&r.ID, &r.RunID, &r.EventKind, &r.Status, &r.AttemptCount, &r.LastStatus, &r.LastError); err != nil {
			return nil, fmt.Errorf("scan delivery result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// RetryPending retries all pending deliveries whose next_retry_at is in the
// past. This is called periodically (e.g. every 30s from a background
// goroutine or sweeper).
func (n *Notifier) RetryPending(ctx context.Context) error {
	rows, err := n.pool.Query(ctx, `
		SELECT id, tenant_id, run_id, event_kind, target_url,
		       attempt_count, max_attempts, COALESCE(last_status_code, 0), COALESCE(last_error, '')
		FROM notification_deliveries
		WHERE status = 'pending' AND next_retry_at <= NOW()
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		return fmt.Errorf("query pending deliveries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, tenantID, runID, eventKind, targetURL, lastErr string
		var attemptCount, maxAttempts, lastStatusCode int

		if err := rows.Scan(&id, &tenantID, &runID, &eventKind, &targetURL, &attemptCount, &maxAttempts, &lastStatusCode, &lastErr); err != nil {
			return fmt.Errorf("scan pending delivery: %w", err)
		}

		event := core.NotifyEvent{Kind: eventKind, RunID: runID}
		statusCode, deliverErr := n.deliver(ctx, targetURL, "", event)

		newCount := attemptCount + 1

		if deliverErr != nil || statusCode >= 500 || statusCode == 429 {
			if newCount >= maxAttempts {
				// Final failure — surface the error.
				errMsg := deliverErr.Error()
				if deliverErr == nil {
					errMsg = fmt.Sprintf("HTTP %d", statusCode)
				}
				n.recordTerminalFailure(ctx, id, statusCode, errMsg)
			} else {
				backoff := time.Duration(math.Min(
					float64(maxBackoff),
					float64(initialBackoff)*math.Pow(2, float64(newCount-1)),
				))
				errMsg := ""
				if deliverErr != nil {
					errMsg = deliverErr.Error()
				} else {
					errMsg = fmt.Sprintf("HTTP %d", statusCode)
				}
				n.recordFailure(ctx, id, statusCode, errMsg, backoff)
			}
		} else if statusCode >= 400 {
			errMsg := fmt.Sprintf("HTTP %d", statusCode)
			n.recordTerminalFailure(ctx, id, statusCode, errMsg)
		} else {
			n.recordSuccess(ctx, id, statusCode)
		}
	}

	return rows.Err()
}

// deliver sends the event as an HTTP POST to the target URL with HMAC signing.
func (n *Notifier) deliver(ctx context.Context, targetURL, signingSecret string, event core.NotifyEvent) (int, error) {
	body, err := json.Marshal(map[string]string{
		"kind":   event.Kind,
		"run_id": event.RunID,
	})
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if signingSecret != "" {
		mac := hmac.New(sha256.New, []byte(signingSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Mework-Signature", "sha256="+sig)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

func (n *Notifier) recordSuccess(ctx context.Context, id string, statusCode int) {
	_, _ = n.pool.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = 'delivered', attempt_count = attempt_count + 1,
		    last_status_code = $1, delivered_at = NOW()
		WHERE id = $2
	`, statusCode, id)
}

func (n *Notifier) recordFailure(ctx context.Context, id string, statusCode int, errMsg string, backoff time.Duration) {
	nextRetry := time.Now().Add(backoff)
	_, _ = n.pool.Exec(ctx, `
		UPDATE notification_deliveries
		SET attempt_count = attempt_count + 1,
		    last_status_code = CASE WHEN $2 <> 0 THEN $2 ELSE last_status_code END,
		    last_error = $3, next_retry_at = $4
		WHERE id = $1
	`, id, statusCode, errMsg, nextRetry)
}

func (n *Notifier) recordTerminalFailure(ctx context.Context, id string, statusCode int, errMsg string) {
	_, _ = n.pool.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = 'failed', attempt_count = attempt_count + 1,
		    last_status_code = CASE WHEN $2 <> 0 THEN $2 ELSE last_status_code END,
		    last_error = $3
		WHERE id = $1
	`, id, statusCode, errMsg)
}

// EnsureTables creates the notification tables if they don't exist.
// This is useful for tests that need to set up notification tracking
// without running the full migration.
func EnsureTables(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS notification_targets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			url TEXT NOT NULL,
			signing_secret TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id)
		);
		CREATE TABLE IF NOT EXISTS notification_deliveries (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			run_id VARCHAR(255) NOT NULL,
			event_kind VARCHAR(50) NOT NULL,
			target_url TEXT NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			attempt_count INT NOT NULL DEFAULT 0,
			max_attempts INT NOT NULL DEFAULT 4,
			last_status_code INT,
			last_error TEXT,
			next_retry_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			delivered_at TIMESTAMPTZ
		);
	`)
	return err
}

// SetTarget configures a notification target for a tenant. This is used by
// admin APIs and tests to configure where notifications are sent.
func SetTarget(ctx context.Context, pool *pgxpool.Pool, tenantID, url, signingSecret string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO notification_targets (tenant_id, url, signing_secret, enabled)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (tenant_id) DO UPDATE SET url = $2, signing_secret = $3, enabled = true
	`, tenantID, url, signingSecret)
	return err
}
