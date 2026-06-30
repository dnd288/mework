package turbo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/platform/secret"
)

// Store provides tenant-scoped database access for the mezon_bots table.
// All query methods enforce WHERE tenant_id = $1 to maintain tenant isolation.
// API keys are sealed with AES-256-GCM at rest and decrypted on retrieval.
type Store struct {
	pool      *pgxpool.Pool
	secretKey string
}

// NewStore creates a Store with the given pgx pool and sealing key.
func NewStore(pool *pgxpool.Pool, secretKey string) *Store {
	return &Store{pool: pool, secretKey: secretKey}
}

// ListByTenant returns all bots for a tenant.
func (s *Store) ListByTenant(ctx context.Context, tenantID string) ([]BotRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, account_id, name, app_id, base_url,
		       status, plan, workspace_id, created_at, updated_at
		FROM mezon_bots
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	defer rows.Close()

	var bots []BotRow
	for rows.Next() {
		var b BotRow
		if err := rows.Scan(&b.ID, &b.TenantID, &b.AccountID, &b.Name, &b.AppID,
			&b.BaseURL, &b.Status, &b.Plan, &b.WorkspaceID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan bot: %w", err)
		}
		bots = append(bots, b)
	}
	return bots, rows.Err()
}

// ListActive returns all active bots across all tenants (for startup recovery).
func (s *Store) ListActive(ctx context.Context) ([]BotRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, account_id, name, app_id, base_url,
		       status, plan, workspace_id, created_at, updated_at
		FROM mezon_bots
		WHERE status = 'active'
		ORDER BY tenant_id, created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list active bots: %w", err)
	}
	defer rows.Close()

	var bots []BotRow
	for rows.Next() {
		var b BotRow
		if err := rows.Scan(&b.ID, &b.TenantID, &b.AccountID, &b.Name, &b.AppID,
			&b.BaseURL, &b.Status, &b.Plan, &b.WorkspaceID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan bot: %w", err)
		}
		bots = append(bots, b)
	}
	return bots, rows.Err()
}

// Get returns a single bot scoped by tenant_id.
func (s *Store) Get(ctx context.Context, tenantID, botID string) (*BotRow, error) {
	var b BotRow
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, account_id, name, app_id, base_url,
		       status, plan, workspace_id, created_at, updated_at
		FROM mezon_bots
		WHERE id = $1 AND tenant_id = $2
	`, botID, tenantID).Scan(
		&b.ID, &b.TenantID, &b.AccountID, &b.Name, &b.AppID,
		&b.BaseURL, &b.Status, &b.Plan, &b.WorkspaceID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("bot not found")
		}
		return nil, fmt.Errorf("get bot: %w", err)
	}
	return &b, nil
}

// Create inserts a new bot, sealing the API key. Returns the created BotRow.
func (s *Store) Create(ctx context.Context, tenantID, accountID string, req *RegisterBotRequest) (*BotRow, error) {
	sealedKey, err := secret.Seal(req.APIKey, s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("seal api key: %w", err)
	}

	plan := req.Plan
	if plan == "" {
		plan = "starter"
	}
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = "https://api.mezon.vn"
	}

	var b BotRow
	err = s.pool.QueryRow(ctx, `
		INSERT INTO mezon_bots (tenant_id, account_id, name, app_id, api_key_enc, base_url, plan, workspace_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, account_id, name, app_id, base_url,
		          status, plan, workspace_id, created_at, updated_at
	`, tenantID, accountID, req.Name, req.AppID, sealedKey, baseURL, plan, req.WorkspaceID).
		Scan(&b.ID, &b.TenantID, &b.AccountID, &b.Name, &b.AppID,
			&b.BaseURL, &b.Status, &b.Plan, &b.WorkspaceID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	return &b, nil
}

// UpdateStatus updates the status field (active/inactive) scoped by tenant.
func (s *Store) UpdateStatus(ctx context.Context, tenantID, botID, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE mezon_bots SET status = $1
		WHERE id = $2 AND tenant_id = $3
	`, status, botID, tenantID)
	if err != nil {
		return fmt.Errorf("update bot status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("bot not found")
	}
	return nil
}

// Delete removes a bot scoped by tenant_id.
func (s *Store) Delete(ctx context.Context, tenantID, botID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM mezon_bots
		WHERE id = $1 AND tenant_id = $2
	`, botID, tenantID)
	if err != nil {
		return fmt.Errorf("delete bot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("bot not found")
	}
	return nil
}

// GetDecryptedKey returns the decrypted API key for a bot, scoped by tenant.
func (s *Store) GetDecryptedKey(ctx context.Context, tenantID, botID string) (string, error) {
	var enc string
	err := s.pool.QueryRow(ctx, `
		SELECT api_key_enc FROM mezon_bots
		WHERE id = $1 AND tenant_id = $2
	`, botID, tenantID).Scan(&enc)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("bot not found")
		}
		return "", fmt.Errorf("get encrypted key: %w", err)
	}

	plaintext, err := secret.Open(enc, s.secretKey)
	if err != nil {
		return "", fmt.Errorf("decrypt api key: %w", err)
	}
	return plaintext, nil
}

// GetDecryptedKeyRaw decrypts the api_key_enc for a given bot ID without
// tenant scoping (used only during startup recovery when the tenant context
// is the bot's own tenant from the DB row).
func (s *Store) GetDecryptedKeyRaw(ctx context.Context, botID string) (string, error) {
	var enc string
	err := s.pool.QueryRow(ctx, `
		SELECT api_key_enc FROM mezon_bots WHERE id = $1
	`, botID).Scan(&enc)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("bot not found")
		}
		return "", fmt.Errorf("get encrypted key: %w", err)
	}

	plaintext, err := secret.Open(enc, s.secretKey)
	if err != nil {
		return "", fmt.Errorf("decrypt api key: %w", err)
	}
	return plaintext, nil
}

// ExistsByAppID checks whether a bot with the given app_id already exists
// in the tenant's scope.
func (s *Store) ExistsByAppID(ctx context.Context, tenantID, appID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM mezon_bots WHERE tenant_id = $1 AND app_id = $2)
	`, tenantID, appID).Scan(&exists)
	return exists, err
}
