package registry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/internal/server/token"
)

var (
	ErrDuplicateCode = errors.New("runtime code already registered for this account")
	ErrNotFound      = errors.New("runtime not found")
)

type Runtime struct {
	ID         string     `json:"id"`
	AccountID  string     `json:"account_id"`
	Code       string     `json:"code"`
	Label      string     `json:"label"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Service struct {
	pool      *pgxpool.Pool
	serverKey string
}

func NewService(pool *pgxpool.Pool, serverKey string) *Service {
	return &Service{
		pool:      pool,
		serverKey: serverKey,
	}
}

// CreateRuntime registers a new runtime and returns its plaintext token.
func (s *Service) CreateRuntime(ctx context.Context, accountID, code, label string) (*Runtime, string, error) {
	if code == "" || label == "" {
		return nil, "", errors.New("code and label are required")
	}

	rawToken, err := token.GenerateRandomToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate token: %w", err)
	}

	tokenLookup := token.ComputeLookup(rawToken, s.serverKey)

	var rt Runtime
	err = s.pool.QueryRow(ctx, `
		INSERT INTO runtimes (account_id, code, label, token_lookup)
		VALUES ($1, $2, $3, $4)
		RETURNING id, account_id, code, label, last_seen_at, status, created_at
	`, accountID, code, label, tokenLookup).Scan(
		&rt.ID, &rt.AccountID, &rt.Code, &rt.Label, &rt.LastSeenAt, &rt.Status, &rt.CreatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique violation
			return nil, "", ErrDuplicateCode
		}
		return nil, "", err
	}

	return &rt, rawToken, nil
}

// ListRuntimes lists all runtimes registered under the account.
func (s *Service) ListRuntimes(ctx context.Context, accountID string) ([]Runtime, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, code, label, last_seen_at, status, created_at
		FROM runtimes
		WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runtimes []Runtime
	for rows.Next() {
		var rt Runtime
		err := rows.Scan(&rt.ID, &rt.AccountID, &rt.Code, &rt.Label, &rt.LastSeenAt, &rt.Status, &rt.CreatedAt)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, rt)
	}

	return runtimes, nil
}

// DeleteRuntime revokes/deletes a runtime. It ensures the runtime belongs to the account.
func (s *Service) DeleteRuntime(ctx context.Context, accountID, id string) error {
	cmd, err := s.pool.Exec(ctx, `
		DELETE FROM runtimes
		WHERE account_id = $1 AND id = $2
	`, accountID, id)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
