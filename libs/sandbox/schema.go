// Package sandbox defines the core types for the sandbox bundle format and
// sandbox lifecycle.
package sandbox

// SandboxBundleMetadata defines the metadata structure that every sandbox
// bundle must carry inside its sandbox.yaml file.
type SandboxBundleMetadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Spec    string `yaml:"spec"`
	Backend string `yaml:"backend"`
	Author  string `yaml:"author,omitempty"`
}
