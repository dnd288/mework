package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"mework/libs/sandbox"
)

// definitionForm is the catalog artifact form used for prebuilt sandbox
// definitions. Definitions reuse the agent-catalog version storage, so
// publishing one introduces no new store or schema migration.
const definitionForm = "definition"

// PublishDefinition publishes a prebuilt sandbox definition as an immutable
// catalog artifact. The definition is validated, the backing agent is created
// on first publish, and the metadata is stored as the version payload. The
// `latest` pointer is moved to the new version by PublishVersion. Republishing
// an existing version is rejected with ErrVersionAlreadyExists rather than
// overwriting the immutable version.
func (s *Service) PublishDefinition(ctx context.Context, meta sandbox.SandboxBundleMetadata) (*AgentVersion, error) {
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("invalid definition: %w", err)
	}

	payload, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal definition: %w", err)
	}

	agentID, err := s.ensureAgent(ctx, meta.Name)
	if err != nil {
		return nil, err
	}

	return s.PublishVersion(ctx, agentID, meta.Version, definitionForm, payload, "")
}

// ResolveDefinition resolves a prebuilt definition by name and a version string
// or moving pointer (e.g. "latest"), unmarshalling the stored payload back into
// SandboxBundleMetadata.
func (s *Service) ResolveDefinition(ctx context.Context, name, versionOrChannel string) (*sandbox.SandboxBundleMetadata, error) {
	v, err := s.Resolve(ctx, name, versionOrChannel)
	if err != nil {
		return nil, err
	}

	var meta sandbox.SandboxBundleMetadata
	if err := json.Unmarshal(v.Payload, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal definition: %w", err)
	}
	return &meta, nil
}

// ensureAgent returns the ID of the agent with the given name, creating it on
// first publish.
func (s *Service) ensureAgent(ctx context.Context, name string) (string, error) {
	a, err := s.LookupAgentByName(ctx, name)
	if err == nil {
		return a.ID, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}

	created, err := s.CreateAgent(ctx, name, "")
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}
	return created.ID, nil
}
