package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"mework/libs/sandbox"
)

// writePrebuiltDir writes a sandbox.yaml with the given content into a fresh
// temp directory and returns the directory path.
func writePrebuiltDir(t *testing.T, yamlBody string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sandbox.yaml"), []byte(yamlBody), 0o600); err != nil {
		t.Fatalf("write sandbox.yaml: %v", err)
	}
	return dir
}

// TestPackPrebuilt_RoundTripsBinding verifies that packing a directory holding a
// valid prebuilt sandbox.yaml produces a bundle whose metadata round-trips the
// engine, backend, and image fields (the prebuilt binding).
//
// RED: PackPrebuilt is not implemented yet — this fails to compile.
func TestPackPrebuilt_RoundTripsBinding(t *testing.T) {
	tests := []struct {
		name        string
		yamlBody    string
		wantEngine  string
		wantBackend string
		wantImage   string
	}{
		{
			name: "local engine no image",
			yamlBody: "name: local-claude\n" +
				"version: 1.0.0\n" +
				"engine: local\n" +
				"backend: claude\n",
			wantEngine:  "local",
			wantBackend: "claude",
			wantImage:   "",
		},
		{
			name: "docker engine pinned image",
			yamlBody: "name: docker-claude\n" +
				"version: 1.0.0\n" +
				"engine: docker\n" +
				"backend: claude\n" +
				"image: mework/claude:1.0.0\n",
			wantEngine:  "docker",
			wantBackend: "claude",
			wantImage:   "mework/claude:1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir := writePrebuiltDir(t, tt.yamlBody)
			outPath := filepath.Join(t.TempDir(), "bundle.zip")

			gotPath, err := PackPrebuilt(srcDir, outPath)
			if err != nil {
				t.Fatalf("PackPrebuilt: %v", err)
			}
			if gotPath == "" {
				t.Fatal("expected non-empty bundle path")
			}

			meta := readBundleMetadata(t, gotPath)
			if meta.Engine != tt.wantEngine {
				t.Errorf("engine = %q, want %q", meta.Engine, tt.wantEngine)
			}
			if meta.Backend != tt.wantBackend {
				t.Errorf("backend = %q, want %q", meta.Backend, tt.wantBackend)
			}
			if meta.Image != tt.wantImage {
				t.Errorf("image = %q, want %q", meta.Image, tt.wantImage)
			}
		})
	}
}

// TestPackPrebuilt_RejectsInvalidDefinition verifies that packing an invalid
// prebuilt definition fails with a validation error before any bundle is
// produced. A docker engine missing an image is the canonical invalid case.
//
// RED: PackPrebuilt is not implemented yet — this fails to compile.
func TestPackPrebuilt_RejectsInvalidDefinition(t *testing.T) {
	tests := []struct {
		name     string
		yamlBody string
	}{
		{
			name: "docker missing image",
			yamlBody: "name: docker-claude\n" +
				"version: 1.0.0\n" +
				"engine: docker\n" +
				"backend: claude\n",
		},
		{
			name: "unknown engine",
			yamlBody: "name: bad\n" +
				"version: 1.0.0\n" +
				"engine: bogus\n" +
				"backend: claude\n",
		},
		{
			name: "missing version",
			yamlBody: "name: local-claude\n" +
				"engine: local\n" +
				"backend: claude\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir := writePrebuiltDir(t, tt.yamlBody)
			outPath := filepath.Join(t.TempDir(), "bundle.zip")

			_, err := PackPrebuilt(srcDir, outPath)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if _, statErr := os.Stat(outPath); statErr == nil {
				t.Errorf("bundle %s should not exist after a validation failure", outPath)
			}
		})
	}
}

// TestStarterDefinitions_Validate verifies that every in-tree starter
// definition parses and passes SandboxBundleMetadata.Validate. The three
// starter definitions are local-claude (local, claude, no image),
// docker-claude (docker, claude, pinned image) and codex-docker (docker, codex,
// pinned image).
//
// RED: the definitions/ directory does not exist yet — these reads fail.
func TestStarterDefinitions_Validate(t *testing.T) {
	root := repoRoot(t)

	tests := []struct {
		name        string
		path        string
		wantEngine  string
		wantBackend string
		wantImage   bool
	}{
		{
			name:        "local-claude",
			path:        filepath.Join(root, "definitions", "local-claude", "sandbox.yaml"),
			wantEngine:  "local",
			wantBackend: "claude",
			wantImage:   false,
		},
		{
			name:        "docker-claude",
			path:        filepath.Join(root, "definitions", "docker-claude", "sandbox.yaml"),
			wantEngine:  "docker",
			wantBackend: "claude",
			wantImage:   true,
		},
		{
			name:        "codex-docker",
			path:        filepath.Join(root, "definitions", "codex-docker", "sandbox.yaml"),
			wantEngine:  "docker",
			wantBackend: "codex",
			wantImage:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read definition: %v", err)
			}
			var meta sandbox.SandboxBundleMetadata
			if err := yaml.Unmarshal(data, &meta); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := meta.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if meta.Engine != tt.wantEngine {
				t.Errorf("engine = %q, want %q", meta.Engine, tt.wantEngine)
			}
			if meta.Backend != tt.wantBackend {
				t.Errorf("backend = %q, want %q", meta.Backend, tt.wantBackend)
			}
			if (meta.Image != "") != tt.wantImage {
				t.Errorf("image present = %v (%q), want present = %v", meta.Image != "", meta.Image, tt.wantImage)
			}
		})
	}
}

// readBundleMetadata extracts and unmarshals sandbox.yaml from a packed bundle
// zip for assertion.
func readBundleMetadata(t *testing.T, zipPath string) sandbox.SandboxBundleMetadata {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open bundle zip: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.Base(f.Name) != "sandbox.yaml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open sandbox.yaml in zip: %v", err)
		}
		defer rc.Close()
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, rerr := rc.Read(buf)
			sb.Write(buf[:n])
			if rerr != nil {
				break
			}
		}
		var meta sandbox.SandboxBundleMetadata
		if err := yaml.Unmarshal([]byte(sb.String()), &meta); err != nil {
			t.Fatalf("unmarshal bundle sandbox.yaml: %v", err)
		}
		return meta
	}
	t.Fatal("sandbox.yaml not found in bundle")
	return sandbox.SandboxBundleMetadata{}
}

// repoRoot walks up from the test working directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (go.mod) not found")
		}
		dir = parent
	}
}
