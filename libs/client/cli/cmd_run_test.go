package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mework/libs/client/runner"
	"mework/libs/shared/config"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// executeRun runs runCmd with the given args, capturing stdout.
func executeRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	runCmd.SetOut(&out)
	runCmd.SetErr(&out)
	runCmd.SetArgs(args)
	runCmd.SetContext(context.Background())
	err := runCmd.RunE(runCmd, args)
	return out.String(), err
}

// offlinePidPath returns the expected path for the offline agent PID file.
func offlinePidPath() string {
	return filepath.Join(config.MeworkDir(), "offline.pid")
}

// fakeOfflineRecord captures the JSON-RPC request received by the fake server.
type fakeOfflineRecord struct {
	mu              sync.Mutex
	receivedRequest json.RawMessage
}

// serveFakeOfflineServer starts a fake offline agent listening on the Unix
// socket derived from wsDir. It accepts a single connection, reads the
// JSON-RPC request, and responds with the given output and exitCode.
// Returns the socket path and a record of the received request.
func serveFakeOfflineServer(t *testing.T, wsDir, output string, exitCode int) (sockPath string, rec *fakeOfflineRecord) {
	t.Helper()
	var err error
	sockPath, err = runner.SocketPath(wsDir)
	if err != nil {
		t.Fatalf("SocketPath(%q): %v", wsDir, err)
	}
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen %s: %v", sockPath, err)
	}
	t.Cleanup(func() {
		listener.Close()
		_ = os.Remove(sockPath)
	})

	rec = &fakeOfflineRecord{}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req json.RawMessage
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			return
		}
		rec.mu.Lock()
		rec.receivedRequest = req
		rec.mu.Unlock()

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"result": map[string]interface{}{
				"output":   output,
				"exitCode": exitCode,
			},
			"id": 1,
		}
		_ = json.NewEncoder(conn).Encode(resp)
	}()

	return sockPath, rec
}

// waitForSocket polls until sockPath exists or the deadline expires.
func waitForSocket(t *testing.T, sockPath string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("socket %s never appeared", sockPath)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestRunCmd_NoAgentRunning verifies that "mework run" fails with a clear
// error when no offline agent is running.  It realises the delta-spec
// scenario "Run task when no agent is running".
func TestRunCmd_NoAgentRunning(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	wsDir := t.TempDir()
	t.Chdir(wsDir)

	_, err := executeRun(t, "hello")
	if err == nil {
		t.Fatal("expected error when no agent is running, got nil")
	}
	if !strings.Contains(err.Error(), "no offline agent running") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "no offline agent running")
	}
}

// TestRunCmd_EmptyInstruction verifies that "mework run" with no args or an
// empty string argument prints an error and exits non-zero.  It realises the
// delta-spec scenario "Run task with empty instruction".
func TestRunCmd_EmptyInstruction(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())

	tests := []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "empty string arg", args: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeRun(t, tt.args...)
			if err == nil {
				t.Fatal("expected error for empty instruction, got nil")
			}
			if !strings.Contains(err.Error(), "instruction is required") {
				t.Errorf("error = %q, want it to contain %q", err.Error(), "instruction is required")
			}
		})
	}
}

// TestRunCmd_InstructionViaStdinNotArgv verifies that the instruction text
// reaches the offline agent inside the JSON-RPC request body (over the Unix
// socket) rather than on a subprocess command line.  This enforces the
// injection-safety invariant at the CLI layer.
func TestRunCmd_InstructionViaStdinNotArgv(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	wsDir := t.TempDir()
	t.Chdir(wsDir)

	sockPath, rec := serveFakeOfflineServer(t, wsDir, "some output", 0)
	waitForSocket(t, sockPath)

	const instruction = "test instruction"
	stdout, err := executeRun(t, instruction)
	if err != nil {
		t.Fatalf("unexpected error: %v (stdout=%q)", err, stdout)
	}

	// The instruction must have been sent inside the JSON-RPC request body
	// (not on the command line or argv of a subprocess).
	rec.mu.Lock()
	reqStr := string(rec.receivedRequest)
	rec.mu.Unlock()
	if !strings.Contains(reqStr, instruction) {
		t.Errorf("instruction not found in JSON-RPC request body:\n  body: %s\n  want instruction: %s", reqStr, instruction)
	}
}

// TestRunCmd_PropagatesExitCode verifies that a non-zero exit code from the
// offline agent is propagated back to the caller.
func TestRunCmd_PropagatesExitCode(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	wsDir := t.TempDir()
	t.Chdir(wsDir)

	sockPath, _ := serveFakeOfflineServer(t, wsDir, "failed output", 42)
	waitForSocket(t, sockPath)

	_, err := executeRun(t, "fail")
	if err == nil {
		t.Fatal("expected error for non-zero exit code, got nil")
	}
	// The error message should reference the exit code.
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error = %q, want it to contain exit code 42", err.Error())
	}
}

// TestRunCmd_OfflinePidFile verifies the offline PID file path resolution and
// lifecycle (write, exist, remove).  The PID file is written by the daemon
// start command and consumed by the stop command.
func TestRunCmd_OfflinePidFile(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	home := config.MeworkDir()

	pidPath := offlinePidPath()
	expectedPath := filepath.Join(home, "offline.pid")
	if pidPath != expectedPath {
		t.Errorf("offlinePidPath() = %q, want %q", pidPath, expectedPath)
	}

	// Simulate PID file lifecycle (written by daemon start, removed by stop).
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(pidPath, []byte("12345\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Errorf("PID file should exist after write")
	}
	if err := os.Remove(pidPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("PID file should be removed after cleanup")
	}
}

// TestRunCmd_SuccessStreamsToStdout verifies that the agent's output is
// printed to stdout when the task succeeds.
func TestRunCmd_SuccessStreamsToStdout(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	wsDir := t.TempDir()
	t.Chdir(wsDir)

	const expectedOutput = "task completed successfully"
	sockPath, _ := serveFakeOfflineServer(t, wsDir, expectedOutput, 0)
	waitForSocket(t, sockPath)

	stdout, err := executeRun(t, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v (stdout=%q)", err, stdout)
	}
	if !strings.Contains(stdout, expectedOutput) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, expectedOutput)
	}
}
