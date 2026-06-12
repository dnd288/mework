package agentrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunResult captures an agent invocation's outcome.
type RunResult struct {
	Output   string
	ExitCode int
	Err      error
}

// Run executes the backend with the prompt fed via STDIN (never as a shell
// argument — ticket content is attacker-controllable, so keeping it out of
// argv/shell avoids command injection). workDir is an isolated per-ticket
// directory. The overall timeout bounds runtime; a zero timeout means 30 min.
func Run(ctx context.Context, b Backend, prompt, workDir string, timeout time.Duration) RunResult {
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return RunResult{Err: fmt.Errorf("create work dir: %w", err)}
	}

	// Each backend reads a prompt from stdin in its non-interactive mode. We use
	// a conservative invocation; specific flags can be added per backend later.
	cmd := exec.CommandContext(runCtx, b.Path)
	cmd.Dir = workDir
	cmd.Stdin = bytes.NewReader([]byte(prompt))

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	res := RunResult{Output: out.String()}
	if err != nil {
		res.Err = err
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	return res
}

// WorkDir returns the isolated working directory for a ticket under the profile.
func WorkDir(profileDir, ticketID string) string {
	return filepath.Join(profileDir, "work", ticketID)
}
