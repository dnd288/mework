package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mework/libs/client/runner"
	"mework/libs/sandbox"
)

// compile-time assertion that the file resolver satisfies the runner contract.
var _ runner.DefinitionResolver = (*FileDefinitionResolver)(nil)

// writeWorkspaceConfig writes body to <dir>/mework.yml, failing the test on
// error. An empty body is still written as an (empty) file when write is true.
func writeWorkspaceConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "mework.yml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write mework.yml: %v", err)
	}
}

// validLocalConfig is a well-formed mework.yml for the local engine: no image is
// required because the local engine ignores it.
const validLocalConfig = `name: local-claude
version: 1.0.0
engine: local
backend: claude
`

// validDockerConfig is a well-formed mework.yml for a container engine, which
// must pin an image.
const validDockerConfig = `name: code-fixer
version: 2.3.4
engine: docker
backend: claude
image: ghcr.io/example/code-fixer:2.3.4
`

func TestFileDefinitionResolver_ResolveDefinition(t *testing.T) {
	tests := []struct {
		name string
		// body is the mework.yml content to write; when write is false no file
		// is created (missing-config case).
		body         string
		write        bool
		wantErrIs    error  // expect errors.Is(err, wantErrIs) when non-nil
		wantDecodeErr bool  // expect a (non-not-found) decode error
		wantMeta     *sandbox.SandboxBundleMetadata
	}{
		{
			// Scenario: Load a local workspace config (local engine).
			name:  "load valid local mework.yml",
			body:  validLocalConfig,
			write: true,
			wantMeta: &sandbox.SandboxBundleMetadata{
				Name:    "local-claude",
				Version: "1.0.0",
				Engine:  "local",
				Backend: "claude",
			},
		},
		{
			// Scenario: Load a local workspace config (container engine pins image).
			name:  "load valid docker mework.yml",
			body:  validDockerConfig,
			write: true,
			wantMeta: &sandbox.SandboxBundleMetadata{
				Name:    "code-fixer",
				Version: "2.3.4",
				Engine:  "docker",
				Backend: "claude",
				Image:   "ghcr.io/example/code-fixer:2.3.4",
			},
		},
		{
			// Scenario: Missing config is reported.
			name:      "missing mework.yml is not found",
			write:     false,
			wantErrIs: runner.ErrDefinitionNotFound,
		},
		{
			// Edge: a corrupt file is distinguishable from a missing one.
			name:          "malformed yaml is a decode error",
			body:          "name: local-claude\n\tengine: : : not yaml\n  ::\n",
			write:         true,
			wantDecodeErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.write {
				writeWorkspaceConfig(t, dir, tt.body)
			}

			res := &FileDefinitionResolver{WorkspaceDir: dir}

			got, err := res.ResolveDefinition(context.Background(), "ignored-ref")

			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				if got != nil {
					t.Errorf("metadata = %+v, want nil on not-found", got)
				}
				return
			case tt.wantDecodeErr:
				if err == nil {
					t.Fatalf("ResolveDefinition: want a decode error, got nil")
				}
				if errors.Is(err, runner.ErrDefinitionNotFound) {
					t.Errorf("malformed yaml mapped to ErrDefinitionNotFound; want a distinct decode error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveDefinition: unexpected error: %v", err)
			}
			if got == nil {
				t.Fatalf("ResolveDefinition returned nil metadata, want %+v", tt.wantMeta)
			}
			if got.Name != tt.wantMeta.Name ||
				got.Version != tt.wantMeta.Version ||
				got.Engine != tt.wantMeta.Engine ||
				got.Backend != tt.wantMeta.Backend ||
				got.Image != tt.wantMeta.Image {
				t.Errorf("metadata = %+v, want %+v", got, tt.wantMeta)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("resolved metadata failed Validate(): %v", err)
			}
		})
	}
}

// TestLoadWorkspaceConfig exercises the loader directly, including the
// not-found sentinel mapping for a missing file.
func TestLoadWorkspaceConfig(t *testing.T) {
	t.Run("missing file maps to ErrDefinitionNotFound", func(t *testing.T) {
		dir := t.TempDir()
		got, err := LoadWorkspaceConfig(dir)
		if !errors.Is(err, runner.ErrDefinitionNotFound) {
			t.Fatalf("error = %v, want errors.Is(runner.ErrDefinitionNotFound)", err)
		}
		if got != nil {
			t.Errorf("metadata = %+v, want nil when no mework.yml", got)
		}
	})

	t.Run("valid file loads engine and backend", func(t *testing.T) {
		dir := t.TempDir()
		writeWorkspaceConfig(t, dir, validLocalConfig)
		got, err := LoadWorkspaceConfig(dir)
		if err != nil {
			t.Fatalf("LoadWorkspaceConfig: %v", err)
		}
		if got.Engine != "local" || got.Backend != "claude" {
			t.Errorf("got engine=%q backend=%q, want local/claude", got.Engine, got.Backend)
		}
	})
}
