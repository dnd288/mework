package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"mework/libs/client/runner"
	"mework/libs/sandbox"
)

// workspaceConfigName is the conventional filename for a workspace-local
// definition that the no-server (local-direct) path resolves.
const workspaceConfigName = "mework.yml"

// LoadWorkspaceConfig reads <dir>/mework.yml and decodes it into sandbox bundle
// metadata. A missing file maps to runner.ErrDefinitionNotFound (with nil
// metadata) so callers can distinguish "no definition here" from a corrupt one;
// any other read or YAML-decode failure is returned as a distinct error.
func LoadWorkspaceConfig(dir string) (*sandbox.SandboxBundleMetadata, error) {
	path := filepath.Join(dir, workspaceConfigName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("resolve %q: %w", path, runner.ErrDefinitionNotFound)
		}
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	var meta sandbox.SandboxBundleMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}
	return &meta, nil
}

// FileDefinitionResolver resolves a definition from a workspace directory's
// mework.yml, giving the no-server path the same metadata a server-resolved
// reference yields.
type FileDefinitionResolver struct {
	// WorkspaceDir is the directory containing mework.yml.
	WorkspaceDir string
}

// compile-time assertion that the resolver satisfies the runner contract.
var _ runner.DefinitionResolver = (*FileDefinitionResolver)(nil)

// ResolveDefinition loads the workspace config. The ref is informational for the
// local path (the file, not the reference, is the source of truth).
func (r *FileDefinitionResolver) ResolveDefinition(_ context.Context, _ string) (*sandbox.SandboxBundleMetadata, error) {
	return LoadWorkspaceConfig(r.WorkspaceDir)
}
