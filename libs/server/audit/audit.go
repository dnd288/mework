// Package audit records actions performed in the system for compliance.
//
// It provides a Service for recording and querying security-relevant actions
// (dispatch, grant issuance, runner enrollment, etc.) in an append-only,
// tenant-scoped audit log backed by Postgres.
//
// Entries are stored with a (tenant_id, seq) primary key so they can be
// returned in append order without sorting. The seq is a BIGSERIAL that
// monotonically increases within each tenant.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ActorType identifies the kind of principal performing an audited action.
type ActorType string

const (
	ActorTypeUser   ActorType = "user"
	ActorTypeSystem ActorType = "system"
	ActorTypeRunner ActorType = "runner"
)

// Entry is a single audit log record.
type Entry struct {
	TenantID   string            `json:"tenant_id"`
	Seq        int64             `json:"seq,omitempty"`
	ActorID    string            `json:"actor_id"`
	ActorType  ActorType         `json:"actor_type"`
	Action     string            `json:"action"`
	TargetType string            `json:"target_type,omitempty"`
	TargetID   string            `json:"target_id,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	RecordedAt time.Time         `json:"recorded_at"`
}

// Standard audit action constants for security-relevant operations.
const (
	ActionDispatchRun      = "dispatch.run"
	ActionGrantIssue       = "grant.issue"
	ActionRunnerEnroll     = "runner.enroll"
	ActionRunnerDeactivate = "runner.deactivate"
	ActionQuotaUpdate      = "quota.update"
	ActionConnectionRotate = "connection.rotate"
)

// Service writes and queries the append-only audit log.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new audit Service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Record writes one entry to the audit log. The seq and recorded_at fields
// are set by the database; caller-supplied values are ignored.
func (s *Service) Record(ctx context.Context, e Entry) error {
	if e.TenantID == "" {
		return errors.New("tenant is required")
	}
	if e.Action == "" {
		return errors.New("action is required")
	}
	if e.ActorID == "" {
		return errors.New("actor is required")
	}

	metadataJSON := []byte("{}")
	if len(e.Metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	if e.ActorType == "" {
		e.ActorType = ActorTypeUser
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log (tenant_id, actor_id, actor_type, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, e.TenantID, e.ActorID, string(e.ActorType), e.Action, e.TargetType, e.TargetID, metadataJSON)
	if err != nil {
		return fmt.Errorf("record audit entry: %w", err)
	}

	return nil
}

// Query returns audit log entries for the given tenant in append order (oldest
// first). It enforces tenant scoping: only entries for the specified tenant are
// returned. Setting limit to 0 returns all entries.
func (s *Service) Query(ctx context.Context, tenantID string, limit int) ([]Entry, error) {
	if tenantID == "" {
		return nil, errors.New("tenant is required")
	}

	query := `
		SELECT tenant_id, seq, actor_id, actor_type, action,
		       COALESCE(target_type, ''), COALESCE(target_id, ''),
		       metadata, recorded_at
		FROM audit_log
		WHERE tenant_id = $1
		ORDER BY seq ASC
	`
	args := []any{tenantID}

	if limit > 0 {
		query += ` LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var metadataJSON []byte
		var actorTypeStr string

		err := rows.Scan(&e.TenantID, &e.Seq, &e.ActorID, &actorTypeStr, &e.Action,
			&e.TargetType, &e.TargetID, &metadataJSON, &e.RecordedAt)
		if err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}

		e.ActorType = ActorType(actorTypeStr)

		if len(metadataJSON) > 0 {
			_ = json.Unmarshal(metadataJSON, &e.Metadata)
		}
		if e.Metadata == nil {
			e.Metadata = make(map[string]any)
		}

		entries = append(entries, e)
	}

	if entries == nil {
		entries = []Entry{} // Return empty slice, not nil.
	}

	return entries, nil
}
