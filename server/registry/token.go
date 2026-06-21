package registry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"mework/server/platform/token"
)

const DefaultRegistrationTokenTTL = 1 * time.Second

type RegistrationToken struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	AccountID  string     `json:"account_id,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

type issueTokenOptions struct {
	accountID string
	ttl       time.Duration
}

func defaultIssueTokenOptions() issueTokenOptions {
	return issueTokenOptions{ttl: DefaultRegistrationTokenTTL}
}

type IssueTokenOption func(*issueTokenOptions)

func WithAccountID(id string) IssueTokenOption {
	return func(o *issueTokenOptions) { o.accountID = id }
}

func WithTTL(ttl time.Duration) IssueTokenOption {
	return func(o *issueTokenOptions) { o.ttl = ttl }
}

func (s *Service) ConsumeAndEnrollRunner(ctx context.Context, rawToken string) (*Runtime, string, error) {
	rec, err := s.LookupRegistrationToken(ctx, rawToken)
	if err != nil {
		return nil, "", err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var consumedID string
	err = tx.QueryRow(ctx, `
		UPDATE registration_tokens SET consumed_at = NOW()
		WHERE id = $1 AND consumed_at IS NULL
		RETURNING id
	`, rec.ID).Scan(&consumedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrInvalidRegistrationToken
		}
		return nil, "", fmt.Errorf("consume token: %w", err)
	}

	code, err := generateRunnerCode()
	if err != nil {
		return nil, "", fmt.Errorf("generate runner code: %w", err)
	}

	rawRT, err := token.GenerateRandomToken()
	if err != nil {
		return nil, "", fmt.Errorf("generate runtime token: %w", err)
	}
	tokenLookup := token.ComputeLookup(rawRT, s.serverKey)

	accountID := rec.AccountID
	if accountID == "" {
		return nil, "", errors.New("registration token has no account")
	}

	var rt Runtime
	err = tx.QueryRow(ctx, `
		INSERT INTO runtimes (tenant_id, account_id, code, label, token_lookup)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, account_id, code, label, last_seen_at, status, created_at
	`, rec.TenantID, accountID, code, "Enrolled runner", tokenLookup).Scan(
		&rt.ID, &rt.TenantID, &rt.AccountID, &rt.Code, &rt.Label, &rt.LastSeenAt, &rt.Status, &rt.CreatedAt,
	)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", fmt.Errorf("commit tx: %w", err)
	}

	return &rt, rawRT, nil
}

func generateRunnerCode() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "runner-" + hex.EncodeToString(b), nil
}
