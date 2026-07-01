package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBackendsIncludesAllSupported(t *testing.T) {
	// The default backend list must cover every supported AI CLI, including the
	// new windows-claude and v0 backends — added without any schema migration
	// (provider-agnostic invariant).
	want := []string{"claude", "codex", "opencode", "windows-claude", "v0"}

	have := make(map[string]bool, len(DefaultBackends))
	for _, name := range DefaultBackends {
		have[name] = true
	}

	for _, tt := range want {
		t.Run(tt, func(t *testing.T) {
			if !have[tt] {
				t.Errorf("DefaultBackends missing %q; got %v", tt, DefaultBackends)
			}
		})
	}
}

func TestDetectFindsBackendOnPath(t *testing.T) {
	dir := t.TempDir()
	// Create a fake executable named like a backend.
	fake := filepath.Join(dir, "claude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	b, ok := Detect([]string{"claude"})
	if !ok || b.Name != "claude" {
		t.Fatalf("expected to detect claude, got ok=%v b=%+v", ok, b)
	}
}

func TestDetectReturnsFalseWhenAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, ok := Detect([]string{"definitely-not-installed-xyz"}); ok {
		t.Fatal("should not detect a missing backend")
	}
}

func TestDetectPrefersFirstAvailable(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"codex", "opencode"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	// claude is absent, so codex (next in order) should win.
	b, ok := Detect([]string{"claude", "codex", "opencode"})
	if !ok || b.Name != "codex" {
		t.Fatalf("expected codex, got ok=%v b=%+v", ok, b)
	}
}
