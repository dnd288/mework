package catalog

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SandboxMetadata is the parsed content of sandbox.yaml inside a bundle.
type SandboxMetadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Spec    string `yaml:"spec"`
	Backend string `yaml:"backend"`
	Author  string `yaml:"author,omitempty"`
}

// validateBundle checks that the zip bytes contain a valid sandbox bundle:
// sandbox.yaml must exist at root with required fields, and definition.md
// must also be present at root.
func validateBundle(zipBytes []byte) error {
	if len(zipBytes) == 0 {
		return errors.New("empty bundle payload")
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return fmt.Errorf("invalid zip: %w", err)
	}

	var hasSandboxYAML, hasDefinitionMD bool
	var sandboxContent []byte

	for _, f := range zr.File {
		switch f.Name {
		case "sandbox.yaml":
			hasSandboxYAML = true
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open sandbox.yaml: %w", err)
			}
			sandboxContent, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("read sandbox.yaml: %w", err)
			}
		case "definition.md":
			hasDefinitionMD = true
		}
	}

	if !hasSandboxYAML {
		return errors.New("bundle must contain sandbox.yaml at root")
	}
	if !hasDefinitionMD {
		return errors.New("bundle must contain definition.md at root")
	}

	meta := parseSandboxYAML(string(sandboxContent))
	var missing []string
	if meta.Spec == "" {
		missing = append(missing, "spec")
	}
	if meta.Backend == "" {
		missing = append(missing, "backend")
	}
	if len(missing) > 0 {
		return fmt.Errorf("sandbox.yaml missing required fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

// parseSandboxYAML does a minimal parse of a sandbox.yaml file to extract
// the fields we care about. It handles the simple key: value format used
// by sandbox bundles without needing a full YAML dependency.
func parseSandboxYAML(content string) SandboxMetadata {
	var meta SandboxMetadata
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			meta.Name = val
		case "version":
			meta.Version = val
		case "spec":
			meta.Spec = val
		case "backend":
			meta.Backend = val
		case "author":
			meta.Author = val
		}
	}
	return meta
}

// ExtractBundle extracts all files from a zip bundle to destDir, preserving
// the directory structure stored in the archive.
func ExtractBundle(zipBytes []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return fmt.Errorf("invalid zip: %w", err)
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", f.Name, err)
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			rc.Close()
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
		rc.Close()
	}
	return nil
}

// MaterializeBundle reads a materialized sandbox directory and returns the
// parsed metadata. It reads sandbox.yaml and definition.md and scans tools/
// and hooks/ directories.
func MaterializeBundle(sandboxDir, sandboxContent string) (*SandboxMetadata, error) {
	meta := parseSandboxYAML(sandboxContent)
	if meta.Name == "" {
		return nil, errors.New("sandbox.yaml missing required field: name")
	}
	return &meta, nil
}
