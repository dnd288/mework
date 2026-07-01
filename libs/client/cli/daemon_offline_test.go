package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executeDaemonStart sets flags on daemonStartCmd and calls RunE directly,
// returning captured stdout and any error.
func executeDaemonStart(t *testing.T, flags map[string]string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	daemonStartCmd.SetOut(&out)
	daemonStartCmd.SetErr(&out)
	daemonStartCmd.SetContext(context.Background())
	for k, v := range flags {
		if err := daemonStartCmd.Flags().Set(k, v); err != nil {
			return "", err
		}
	}
	err := daemonStartCmd.RunE(daemonStartCmd, []string{})
	return out.String(), err
}

// TestStartOfflineMissingWorkspace verifies that --offline without --workspace
// produces a clear error. This realises the "Start offline agent without
// workspace" scenario from the offline-agent delta spec.
func TestStartOfflineMissingWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	_, err := executeDaemonStart(t, map[string]string{"offline": "true"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "workspace is required") {
		t.Errorf("error = %q, want substring %q", err.Error(), "workspace is required")
	}
}

// TestStartOfflineBadWorkspaceDir verifies that --offline with a nonexistent
// workspace directory produces a clear error. This realises the "Start offline
// agent when workspace has no mework.yml" scenario (dir does not exist at all).
func TestStartOfflineBadWorkspaceDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	_, err := executeDaemonStart(t, map[string]string{
		"offline":   "true",
		"workspace": "/nonexistent/path",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no such directory") {
		t.Errorf("error = %q, want substring %q", err.Error(), "no such directory")
	}
}

// TestStartOfflineNoMeworkYml verifies that --offline with a valid workspace
// directory that lacks a mework.yml produces a clear error. This realises the
// "Start offline agent when workspace has no mework.yml" scenario (dir exists
// but config missing).
func TestStartOfflineNoMeworkYml(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)
	dir := t.TempDir()

	_, err := executeDaemonStart(t, map[string]string{
		"offline":   "true",
		"workspace": dir,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no mework.yml found in workspace") {
		t.Errorf("error = %q, want substring %q", err.Error(), "no mework.yml found in workspace")
	}
}

// TestStartOfflineValidWorkspace verifies that --offline with a valid workspace
// containing a correct mework.yml passes validation and does not error.
// Because the actual daemon would block waiting for tasks, the test uses a
// cancellable context so that runOfflineForeground returns cleanly. This
// realises the "Start offline agent with valid workspace" scenario.
func TestStartOfflineValidWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	ymlPath := filepath.Join(dir, "mework.yml")
	if err := os.WriteFile(ymlPath, []byte("engine: local\nbackend: echo\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Use a cancellable context so RunE does not block forever.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonStartCmd.SetContext(ctx)

	_, err := executeDaemonStart(t, map[string]string{
		"offline":   "true",
		"workspace": dir,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStartOfflineRejectsNonLocalEngine verifies that a mework.yml specifying a
// non-local engine (e.g. docker) is rejected in offline mode. This realises the
// "Resolve mework.yml with unsupported engine" scenario.
func TestStartOfflineRejectsNonLocalEngine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEWORK_HOME", home)

	dir := t.TempDir()
	ymlPath := filepath.Join(dir, "mework.yml")
	if err := os.WriteFile(ymlPath, []byte("engine: docker\nbackend: claude\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := executeDaemonStart(t, map[string]string{
		"offline":   "true",
		"workspace": dir,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "only 'local' engine") {
		t.Errorf("error = %q, want substring %q", err.Error(), "only 'local' engine")
	}
}

// TestRunCommandRegistered verifies that registerCommands() wires the run
// command into the root CLI. This realises task 4.1.
func TestRunCommandRegistered(t *testing.T) {
	// registerCommands() is invoked by root.go's init() when the package loads,
	// so we simply inspect what is already registered.
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "run" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'run' subcommand to be registered in rootCmd")
	}
}
