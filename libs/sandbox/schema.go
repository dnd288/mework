// Package sandbox defines the core types for the sandbox bundle format and
// sandbox lifecycle.
package sandbox

import (
	"fmt"

	"mework/libs/shared/core"
	"mework/libs/shared/policy"
)

// SandboxBundleMetadata defines the metadata structure that every sandbox
// bundle must carry inside its sandbox.yaml file. When used as a prebuilt
// sandbox definition it also binds an engine, a pinned image (for container
// engines), and resource limits into a ready-to-run combo.
type SandboxBundleMetadata struct {
	Name           string               `yaml:"name" json:"name"`
	Version        string               `yaml:"version" json:"version"`
	Spec           string               `yaml:"spec,omitempty" json:"spec,omitempty"`
	Engine         string               `yaml:"engine,omitempty" json:"engine,omitempty"`
	Backend        string               `yaml:"backend" json:"backend"`
	Image          string               `yaml:"image,omitempty" json:"image,omitempty"`
	ResourceLimits *core.ResourceLimits `yaml:"resourceLimits,omitempty" json:"resourceLimits,omitempty"`
	Author         string               `yaml:"author,omitempty" json:"author,omitempty"`
	Policy         *policy.Policy       `yaml:"policy,omitempty" json:"policy,omitempty"`
}

// knownEngines is the allowlist of engines a definition may select. Adding a
// new engine extends this list only — it requires no schema migration because
// the engine is plain metadata that rides the existing catalog artifact.
var knownEngines = map[string]bool{
	"local":      true,
	"docker":     true,
	"cloudflare": true,
	"custom":     true,
}

// containerEngines are engines that materialize from a pre-baked image. Such
// engines require a pinned image so the sandbox installs nothing at run time.
// Engines absent from this set (e.g. "local") ignore the image field.
var containerEngines = map[string]bool{
	"docker":     true,
	"cloudflare": true,
	"custom":     true,
}

// UsesImage reports whether the definition's engine materializes from a
// pre-baked image (a container engine). The local engine does not, so its
// image field is ignored at run time.
func (m SandboxBundleMetadata) UsesImage() bool {
	return containerEngines[m.Engine]
}

// Validate checks that the definition is well-formed: name and version are
// required, the engine must be a known engine, the backend must be non-empty,
// and container engines must pin an image. The local engine ignores the image.
func (m SandboxBundleMetadata) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if !knownEngines[m.Engine] {
		return fmt.Errorf("unknown engine %q", m.Engine)
	}
	if m.Backend == "" {
		return fmt.Errorf("backend is required")
	}
	if containerEngines[m.Engine] && m.Image == "" {
		return fmt.Errorf("engine %q requires a pinned image", m.Engine)
	}
	return nil
}
