package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("profile name already exists for this account")
	ErrNotFound      = errors.New("profile not found")
	ErrBodyTooLarge  = errors.New("profile body exceeds 64KB limit")
)

type Profile struct {
	ID             string         `json:"id"`
	AccountID      string         `json:"account_id"`
	Name           string         `json:"name"`
	Body           string         `json:"body"`
	BackendHint    string         `json:"backend_hint,omitempty"`
	Harness        string         `json:"harness,omitempty"`
	WorkflowConfig map[string]any `json:"workflow_config"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type CreateProfileRequest struct {
	Name           string         `json:"name"`
	Body           string         `json:"body"`
	BackendHint    string         `json:"backend_hint"`
	Harness        string         `json:"harness"`
	WorkflowConfig map[string]any `json:"workflow_config"`
}

type UpdateProfileRequest struct {
	Body           string         `json:"body"`
	BackendHint    string         `json:"backend_hint"`
	Harness        string         `json:"harness"`
	WorkflowConfig map[string]any `json:"workflow_config"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// CreateProfile creates a new profile for the account.
func (s *Service) CreateProfile(ctx context.Context, accountID string, req CreateProfileRequest) (*Profile, error) {
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.Body == "" {
		return nil, errors.New("body is required")
	}
	if len(req.Body) > 65536 {
		return nil, ErrBodyTooLarge
	}

	if req.WorkflowConfig == nil {
		req.WorkflowConfig = make(map[string]any)
	}

	var p Profile
	var createdAt, updatedAt time.Time

	err := s.pool.QueryRow(ctx, `
		INSERT INTO profiles (account_id, name, body, backend_hint, harness, workflow_config)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, account_id, name, body, backend_hint, harness, workflow_config, created_at, updated_at
	`, accountID, req.Name, req.Body, req.BackendHint, req.Harness, req.WorkflowConfig).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Body, &p.BackendHint, &p.Harness, &p.WorkflowConfig, &createdAt, &updatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateName
		}
		return nil, err
	}

	p.CreatedAt = createdAt.Format(time.RFC3339)
	p.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &p, nil
}

// GetProfile returns a profile by name for the account.
func (s *Service) GetProfile(ctx context.Context, accountID, name string) (*Profile, error) {
	var p Profile
	var createdAt, updatedAt time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, name, body, backend_hint, harness, workflow_config, created_at, updated_at
		FROM profiles
		WHERE account_id = $1 AND name = $2
	`, accountID, name).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Body, &p.BackendHint, &p.Harness, &p.WorkflowConfig, &createdAt, &updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	p.CreatedAt = createdAt.Format(time.RFC3339)
	p.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &p, nil
}

// ListProfiles lists all profiles for the account.
func (s *Service) ListProfiles(ctx context.Context, accountID string) ([]Profile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, name, body, backend_hint, harness, workflow_config, created_at, updated_at
		FROM profiles
		WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		var p Profile
		var createdAt, updatedAt time.Time
		err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Body, &p.BackendHint, &p.Harness, &p.WorkflowConfig, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		p.CreatedAt = createdAt.Format(time.RFC3339)
		p.UpdatedAt = updatedAt.Format(time.RFC3339)
		profiles = append(profiles, p)
	}

	return profiles, nil
}

// UpdateProfile updates the profile's fields and sets updated_at.
func (s *Service) UpdateProfile(ctx context.Context, accountID, name string, req UpdateProfileRequest) (*Profile, error) {
	if req.Body == "" {
		return nil, errors.New("body is required")
	}
	if len(req.Body) > 65536 {
		return nil, ErrBodyTooLarge
	}

	if req.WorkflowConfig == nil {
		req.WorkflowConfig = make(map[string]any)
	}

	var p Profile
	var createdAt, updatedAt time.Time

	err := s.pool.QueryRow(ctx, `
		UPDATE profiles
		SET body = $1, backend_hint = $2, harness = $3, workflow_config = $4, updated_at = NOW()
		WHERE account_id = $5 AND name = $6
		RETURNING id, account_id, name, body, backend_hint, harness, workflow_config, created_at, updated_at
	`, req.Body, req.BackendHint, req.Harness, req.WorkflowConfig, accountID, name).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Body, &p.BackendHint, &p.Harness, &p.WorkflowConfig, &createdAt, &updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	p.CreatedAt = createdAt.Format(time.RFC3339)
	p.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &p, nil
}

// DeleteProfile deletes a profile, ensuring it belongs to the account.
func (s *Service) DeleteProfile(ctx context.Context, accountID, name string) error {
	cmd, err := s.pool.Exec(ctx, `
		DELETE FROM profiles
		WHERE account_id = $1 AND name = $2
	`, accountID, name)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
