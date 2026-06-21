package registry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/platform/token"
)

var (
	ErrDuplicateCode            = errors.New("runtime code already registered for this account")
	ErrNotFound                 = errors.New("runtime not found")
	ErrInvalidRegistrationToken = errors.New("invalid registration token")
)

// DefaultTenantID is the fixed tenant every pre-existing row is backfilled to by the
// tenancy migration. Until the authenticated credential carries a tenant (a later
// unit), tenant-scoped HTTP handlers operate within this default boundary so existing
// behavior is preserved.
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

// Tenant is the isolation boundary primitive: every tenant-scoped resource is
// keyed by its tenant so cross-tenant access can be denied.
type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Runtime struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
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

// RegisterTenant allocates a fresh, isolated tenant namespace and returns it.
// Each registration yields a stable, distinct TenantID; no resources are shared
// with any existing tenant.
func (s *Service) RegisterTenant(ctx context.Context, name string) (*Tenant, error) {
	if name == "" {
		return nil, errors.New("tenant name is required")
	}

	var ten Tenant
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenants (name)
		VALUES ($1)
		RETURNING id, name
	`, name).Scan(&ten.ID, &ten.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to register tenant: %w", err)
	}

	return &ten, nil
}

// IssueRegistrationToken mints a registration token bound to the given tenant and
// returns its plaintext value. Only the token's HMAC lookup is stored, recording the
// owning TenantID; the plaintext is shown once to the caller and never persisted.
// Optional parameters via IssueTokenOption set the token's account_id and TTL.
func (s *Service) IssueRegistrationToken(ctx context.Context, tenant Tenant, opts ...IssueTokenOption) (string, error) {
	if tenant.ID == "" {
		return "", errors.New("tenant is required")
	}

	o := defaultIssueTokenOptions()
	for _, fn := range opts {
		fn(&o)
	}

	var accountIDParam any
	if o.accountID != "" {
		accountIDParam = o.accountID
	}

	rawToken, err := token.GenerateRandomToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate registration token: %w", err)
	}

	lookup := token.ComputeLookup(rawToken, s.serverKey)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO registration_tokens (tenant_id, account_id, token_lookup, expires_at)
		VALUES ($1, $2, $3, NOW() + $4::interval)
	`, tenant.ID, accountIDParam, lookup, fmt.Sprintf("%d seconds", int(o.ttl.Seconds())))
	if err != nil {
		return "", fmt.Errorf("failed to record registration token: %w", err)
	}

	return rawToken, nil
}

// LookupRegistrationToken resolves a plaintext registration token to its stored record,
// exposing the owning TenantID and AccountID. An unknown, expired, or already-consumed
// token yields ErrInvalidRegistrationToken.
func (s *Service) LookupRegistrationToken(ctx context.Context, rawToken string) (*RegistrationToken, error) {
	lookup := token.ComputeLookup(rawToken, s.serverKey)
	var rec RegistrationToken
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(account_id::text, ''), expires_at, consumed_at
		FROM registration_tokens
		WHERE token_lookup = $1 AND consumed_at IS NULL AND expires_at > NOW()
	`, lookup).Scan(&rec.ID, &rec.TenantID, &rec.AccountID, &rec.ExpiresAt, &rec.ConsumedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidRegistrationToken
		}
		return nil, err
	}
	return &rec, nil
}

// EnrollRunner enrolls a new runner using a registration token. The runner inherits the
// token's tenant — never a caller-supplied one — so it lands in the token's tenant by
// construction and cross-tenant enrollment is denied. An unknown token is rejected with
// ErrInvalidRegistrationToken.
func (s *Service) EnrollRunner(ctx context.Context, rawToken, accountID, code, label string) (*Runtime, error) {
	rec, err := s.LookupRegistrationToken(ctx, rawToken)
	if err != nil {
		return nil, err
	}

	rt, _, err := s.CreateRuntime(ctx, Tenant{ID: rec.TenantID}, accountID, code, label)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// CreateRuntime registers a new runtime under the given tenant and returns its
// plaintext token. The runtime is stamped with the caller's TenantID so it is
// only ever visible within that tenant's boundary.
func (s *Service) CreateRuntime(ctx context.Context, tenant Tenant, accountID, code, label string) (*Runtime, string, error) {
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
		INSERT INTO runtimes (tenant_id, account_id, code, label, token_lookup)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, account_id, code, label, last_seen_at, status, created_at
	`, tenant.ID, accountID, code, label, tokenLookup).Scan(
		&rt.ID, &rt.TenantID, &rt.AccountID, &rt.Code, &rt.Label, &rt.LastSeenAt, &rt.Status, &rt.CreatedAt,
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

// ListRunners lists the caller's runners within its tenant boundary. Results are
// scoped by BOTH tenant and account: another tenant's runners are never visible, and
// within a tenant a caller still only sees its own account's runners (so callers that
// share the default tenant during migration do not observe each other's runtimes).
func (s *Service) ListRunners(ctx context.Context, tenant Tenant, accountID string) ([]Runtime, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, account_id, code, label, last_seen_at, status, created_at
		FROM runtimes
		WHERE tenant_id = $1 AND account_id = $2
		ORDER BY created_at DESC
	`, tenant.ID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runtimes []Runtime
	for rows.Next() {
		var rt Runtime
		err := rows.Scan(&rt.ID, &rt.TenantID, &rt.AccountID, &rt.Code, &rt.Label, &rt.LastSeenAt, &rt.Status, &rt.CreatedAt)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, rt)
	}

	return runtimes, nil
}

// DeleteRuntime revokes/deletes a runtime within the caller's tenant AND account
// boundary. A runtime owned by another tenant — or by another account within the same
// tenant — is invisible: deleting it returns ErrNotFound rather than a forbidden error,
// so cross-tenant and cross-account access are denied by construction.
func (s *Service) DeleteRuntime(ctx context.Context, tenant Tenant, accountID, id string) error {
	cmd, err := s.pool.Exec(ctx, `
		DELETE FROM runtimes
		WHERE tenant_id = $1 AND account_id = $2 AND id = $3
	`, tenant.ID, accountID, id)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
