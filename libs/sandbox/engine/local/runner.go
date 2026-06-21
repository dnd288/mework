// Package local implements the ports.SandboxDriver interface as a host
// subprocess — the same behaviour as the current agentrun.Run call. It
// provides NO host isolation and is intended for trusted agents only.
package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"mework/libs/sandbox/agent"
	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// Driver implements ports.SandboxDriver for the local subprocess engine.
type Driver struct{}

// New creates a new local Driver.
func New() *Driver { return &Driver{} }

// Caps returns the capabilities of this driver.
func (d *Driver) Caps() core.SandboxCaps {
	return core.SandboxCaps{
		SupportsGPU: true,
		SupportsNet: true,
		IsIsolated:  false,
		IsRemote:    false,
		DriverName:  "local",
	}
}

// localSandbox is a running local subprocess sandbox.
type localSandbox struct {
	id      string
	workDir string
}

func (s *localSandbox) ID() string { return s.id }

// Exec runs a command as a host subprocess. Never places the prompt on the
// command line (injection-safety invariant).
func (s *localSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = s.workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return -1, err
	}
	return 0, nil
}

// Mount is a no-op for the local driver — the host filesystem is already visible.
func (s *localSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	return nil
}

// Signals is a no-op for the local driver.
func (s *localSandbox) Signals(ctx context.Context, sig string) error {
	return nil
}

// Start creates a working directory and returns a sandbox. No process is
// started until Exec is called.
func (d *Driver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	workDir := spec.SandboxID
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "mework-sandbox", spec.AgentID)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	return &localSandbox{
		id:      spec.SandboxID,
		workDir: workDir,
	}, nil
}

// Stop is a no-op for the local driver.
func (d *Driver) Stop(ctx context.Context, sandboxID string) error { return nil }

// Destroy is a no-op for the local driver.
func (d *Driver) Destroy(ctx context.Context, sandboxID string) error { return nil }

// RunResult captures an agent invocation's outcome.
type RunResult struct {
	Output   string
	ExitCode int
	Err      error
}

// Run executes the backend with the prompt fed via STDIN (never as a shell
// argument). workDir is an isolated per-ticket directory. The overall timeout
// bounds runtime; a zero timeout means 30 min.
//
// This is the legacy entry point, kept for backward compatibility.
// New code should use Driver.Run (or sandbox/runtime.Manager).
func Run(ctx context.Context, b agent.Backend, prompt, workDir string, timeout time.Duration) RunResult {
	drv := New()
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return RunResult{Err: fmt.Errorf("create work dir: %w", err)}
	}

	s, err := drv.Start(runCtx, core.RunSpec{
		AgentID:     b.Name,
		BackendPath: b.Path,
		BackendName: b.Name,
		SandboxID:   workDir,
	})
	if err != nil {
		return RunResult{Err: fmt.Errorf("start: %w", err)}
	}
	defer func() { _ = drv.Destroy(context.Background(), s.ID()) }()

	var stdout, stderr bytes.Buffer
	exitCode, execErr := s.Exec(runCtx, []string{b.Path}, bytes.NewReader([]byte(prompt)), &stdout, &stderr)

	res := RunResult{Output: stdout.String() + stderr.String(), ExitCode: exitCode}
	if execErr != nil {
		res.Err = execErr
		if exitCode <= 0 {
			res.ExitCode = -1
		}
	}
	return res
}

// WorkDir returns the isolated working directory for a ticket under the profile.
func WorkDir(profileDir, ticketID string) string {
	return filepath.Join(profileDir, "work", ticketID)
}
