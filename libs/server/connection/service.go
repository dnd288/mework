package connection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/platform/secret"
)

var (
	ErrDuplicateConnection = errors.New("connection already exists for this provider")
	ErrNotFound            = errors.New("connection not found")
)

type Connection struct {
	ID            string         `json:"id"`
	AccountID     string         `json:"account_id"`
	ProviderCode  string         `json:"provider_code"`
	WebhookSecret string         `json:"webhook_secret,omitempty"`
	Config        map[string]any `json:"config"`
	CreatedAt     string         `json:"created_at"`
}

type CreateConnectionRequest struct {
	ProviderCode  string         `json:"provider_code"`
	ProviderToken string         `json:"provider_token"`
	WebhookSecret string         `json:"webhook_secret"`
	Config        map[string]any `json:"config"`
}

type Service struct {
	pool      *pgxpool.Pool
	secretKey string
}

func NewService(pool *pgxpool.Pool, secretKey string) *Service {
	return &Service{
		pool:      pool,
		secretKey: secretKey,
	}
}

// CreateConnection upserts the connection for the account and provider code.
func (s *Service) CreateConnection(ctx context.Context, accountID, providerCode, providerToken, webhookSecret string, config map[string]any) (*Connection, error) {
	if providerCode == "" {
		return nil, errors.New("provider_code is required")
	}
	if providerToken == "" {
		return nil, errors.New("provider_token is required")
	}

	encryptedToken, err := secret.Seal(providerToken, s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt provider token: %w", err)
	}

	if config == nil {
		config = make(map[string]any)
	}

	var conn Connection
	var createdAt time.Time

	err = s.pool.QueryRow(ctx, `
		INSERT INTO provider_connections (account_id, provider_code, mcp_auth_enc, webhook_secret, config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, provider_code) DO UPDATE SET
			mcp_auth_enc = EXCLUDED.mcp_auth_enc,
			webhook_secret = EXCLUDED.webhook_secret,
			config = EXCLUDED.config
		RETURNING id, account_id, provider_code, webhook_secret, config, created_at
	`, accountID, providerCode, encryptedToken, webhookSecret, config).Scan(
		&conn.ID, &conn.AccountID, &conn.ProviderCode, &conn.WebhookSecret, &conn.Config, &createdAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateConnection
		}
		return nil, err
	}

	conn.CreatedAt = createdAt.Format(time.RFC3339)
	return &conn, nil
}

// GetConnection returns the connection by provider code, ensuring it belongs to the account.
// It never returns the decrypted provider token.
func (s *Service) GetConnection(ctx context.Context, accountID, providerCode string) (*Connection, error) {
	var conn Connection
	var createdAt time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, provider_code, webhook_secret, config, created_at
		FROM provider_connections
		WHERE account_id = $1 AND provider_code = $2
	`, accountID, providerCode).Scan(
		&conn.ID, &conn.AccountID, &conn.ProviderCode, &conn.WebhookSecret, &conn.Config, &createdAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	conn.CreatedAt = createdAt.Format(time.RFC3339)
	return &conn, nil
}

// ListConnections lists all connections for the account.
func (s *Service) ListConnections(ctx context.Context, accountID string) ([]Connection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, provider_code, webhook_secret, config, created_at
		FROM provider_connections
		WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []Connection
	for rows.Next() {
		var conn Connection
		var createdAt time.Time
		err := rows.Scan(&conn.ID, &conn.AccountID, &conn.ProviderCode, &conn.WebhookSecret, &conn.Config, &createdAt)
		if err != nil {
			return nil, err
		}
		conn.CreatedAt = createdAt.Format(time.RFC3339)
		conns = append(conns, conn)
	}

	return conns, nil
}

// DeleteConnection deletes a connection, ensuring it belongs to the account.
func (s *Service) DeleteConnection(ctx context.Context, accountID, providerCode string) error {
	cmd, err := s.pool.Exec(ctx, `
		DELETE FROM provider_connections
		WHERE account_id = $1 AND provider_code = $2
	`, accountID, providerCode)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetDecryptedToken returns the decrypted token for the connection.
// This is used internally server-side for write-back and other actions.
func (s *Service) GetDecryptedToken(ctx context.Context, accountID, providerCode string) (string, error) {
	var encryptedToken string
	err := s.pool.QueryRow(ctx, `
		SELECT mcp_auth_enc
		FROM provider_connections
		WHERE account_id = $1 AND provider_code = $2
	`, accountID, providerCode).Scan(&encryptedToken)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	if encryptedToken == "" {
		return "", errors.New("provider token is empty")
	}

	decrypted, err := secret.Open(encryptedToken, s.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt provider token: %w", err)
	}

	return decrypted, nil
}
