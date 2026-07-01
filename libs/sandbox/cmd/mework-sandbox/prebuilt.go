package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"mework/libs/sandbox"
)

// PackPrebuilt loads the prebuilt sandbox.yaml from srcDir, validates it as a
// prebuilt definition (engine/backend binding plus pinned image for container
// engines), and only then packs the directory into a bundle. Validation happens
// before any bundle is written, so an invalid definition leaves no output file.
func PackPrebuilt(srcDir, outputPath string) (string, error) {
	sandboxYAML := filepath.Join(srcDir, "sandbox.yaml")
	data, err := os.ReadFile(sandboxYAML)
	if err != nil {
		return "", fmt.Errorf("read sandbox.yaml: %w", err)
	}

	var meta sandbox.SandboxBundleMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parse sandbox.yaml: %w", err)
	}
	if err := meta.Validate(); err != nil {
		return "", fmt.Errorf("invalid prebuilt definition: %w", err)
	}

	return PackBundle(srcDir, outputPath)
}

// PushPrebuilt packs a prebuilt definition directory and publishes the bundle to
// the agent catalog. It reuses PackPrebuilt (validate-then-pack) and the
// existing PushBundle plumbing so a prebuilt definition is published as an
// ordinary agent-catalog artifact.
func PushPrebuilt(serverURL, srcDir, outputPath string) error {
	data, err := os.ReadFile(filepath.Join(srcDir, "sandbox.yaml"))
	if err != nil {
		return fmt.Errorf("read sandbox.yaml: %w", err)
	}
	var meta sandbox.SandboxBundleMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse sandbox.yaml: %w", err)
	}
	bundlePath, err := PackPrebuilt(srcDir, outputPath)
	if err != nil {
		return err
	}
	return PushBundle(serverURL, meta.Name, meta.Version, bundlePath)
}
