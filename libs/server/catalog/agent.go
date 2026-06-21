package catalog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrVersionAlreadyExists is returned when publishing a version that already exists
	// for the same agent (unique constraint on agent_id + version).
	ErrVersionAlreadyExists = errors.New("agent version already exists")
)

// Agent represents a named agent catalog entry.
type Agent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// AgentVersion is an immutable versioned artifact belonging to an agent.
type AgentVersion struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Version   string    `json:"version"`
	Form      string    `json:"form"`
	Payload   []byte    `json:"payload,omitempty"`
	Reference string    `json:"reference,omitempty"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
}

// Service is the store/DAO layer for agent catalog operations.
// It embeds/reuses the same *pgxpool.Pool pattern as the existing Service.
func (s *Service) CreateAgent(ctx context.Context, name, description string) (*Agent, error) {
	var a Agent
	err := s.pool.QueryRow(ctx, `
		INSERT INTO agents (name, description)
		VALUES ($1, $2)
		RETURNING id, name, description, created_at
	`, name, description).Scan(&a.ID, &a.Name, &a.Description, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// PublishVersion inserts a new immutable version for the given agent. The
// version string must be unique per agent (immutability). The `latest` pointer
// is automatically moved to this new version.
func (s *Service) PublishVersion(ctx context.Context, agentID, version, form string, payload []byte, reference string) (*AgentVersion, error) {
	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var v AgentVersion
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_versions (agent_id, version, form, payload, reference, checksum)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, agent_id, version, form, payload, reference, checksum, created_at
	`, agentID, version, form, payload, reference, checksum).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Form, &v.Payload, &v.Reference, &v.Checksum, &v.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrVersionAlreadyExists
		}
		return nil, fmt.Errorf("insert version: %w", err)
	}

	// Move the latest pointer to the newly published version.
	_, err = tx.Exec(ctx, `
		INSERT INTO agent_pointers (agent_id, channel, version_id)
		VALUES ($1, 'latest', $2)
		ON CONFLICT (agent_id, channel) DO UPDATE SET version_id = $2
	`, agentID, v.ID)
	if err != nil {
		return nil, fmt.Errorf("update latest pointer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &v, nil
}

// Resolve resolves an agent version by agent name and a version string or
// channel name. If versionOrChannel matches a channel pointer (e.g. "latest",
// "stable"), the pointed version is returned. Otherwise it attempts an exact
// version lookup. If "latest" has no explicit pointer, the most recently
// published version is returned as a sensible fallback.
func (s *Service) Resolve(ctx context.Context, agentName, versionOrChannel string) (*AgentVersion, error) {
	// Resolve agent name to ID.
	var agentID string
	err := s.pool.QueryRow(ctx, `SELECT id FROM agents WHERE name = $1`, agentName).Scan(&agentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lookup agent: %w", err)
	}

	// Try pointer/channel lookup first.
	var versionID string
	err = s.pool.QueryRow(ctx, `
		SELECT version_id FROM agent_pointers WHERE agent_id = $1 AND channel = $2
	`, agentID, versionOrChannel).Scan(&versionID)
	if err == nil {
		// Pointer found — fetch and return the pointed version.
		return s.getVersionByID(ctx, versionID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup pointer: %w", err)
	}

	// Not a channel pointer — try exact version match.
	v, err := s.getVersionByAgentAndVersion(ctx, agentID, versionOrChannel)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// As a fallback for the "latest" channel when no pointer has been
	// explicitly set (e.g. after a data migration that only creates
	// agents and versions), return the most recently published version.
	if versionOrChannel == "latest" {
		return s.getLatestVersion(ctx, agentID)
	}

	return nil, ErrNotFound
}

// SetChannelPointer sets a named pointer to a specific version. If the
// (agent_id, channel) pair already exists it is updated.
func (s *Service) SetChannelPointer(ctx context.Context, agentID, channel, versionID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_pointers (agent_id, channel, version_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (agent_id, channel) DO UPDATE SET version_id = $3
	`, agentID, channel, versionID)
	if err != nil {
		return fmt.Errorf("set channel pointer: %w", err)
	}
	return nil
}

// LookupAgentByName returns an agent by its name.
func (s *Service) LookupAgentByName(ctx context.Context, name string) (*Agent, error) {
	if s.pool == nil {
		return nil, ErrNotFound
	}
	var a Agent
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, created_at FROM agents WHERE name = $1
	`, name).Scan(&a.ID, &a.Name, &a.Description, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lookup agent by name: %w", err)
	}
	return &a, nil
}

// ListAgents returns all agent catalog entries.
func (s *Service) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, created_at FROM agents ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return agents, nil
}

// getVersionByID fetches a single agent version by its primary key.
func (s *Service) getVersionByID(ctx context.Context, versionID string) (*AgentVersion, error) {
	var v AgentVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, agent_id, version, form, payload, reference, checksum, created_at
		FROM agent_versions WHERE id = $1
	`, versionID).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Form, &v.Payload, &v.Reference, &v.Checksum, &v.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get version by id: %w", err)
	}
	return &v, nil
}

// getVersionByAgentAndVersion fetches a version by agent ID and version string.
func (s *Service) getVersionByAgentAndVersion(ctx context.Context, agentID, version string) (*AgentVersion, error) {
	var v AgentVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, agent_id, version, form, payload, reference, checksum, created_at
		FROM agent_versions WHERE agent_id = $1 AND version = $2
	`, agentID, version).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Form, &v.Payload, &v.Reference, &v.Checksum, &v.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get version by agent+version: %w", err)
	}
	return &v, nil
}

// getLatestVersion returns the most recently published version for an agent.
// This is the fallback when no explicit "latest" pointer exists.
func (s *Service) getLatestVersion(ctx context.Context, agentID string) (*AgentVersion, error) {
	var v AgentVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, agent_id, version, form, payload, reference, checksum, created_at
		FROM agent_versions WHERE agent_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, agentID).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Form, &v.Payload, &v.Reference, &v.Checksum, &v.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get latest version: %w", err)
	}
	return &v, nil
}
