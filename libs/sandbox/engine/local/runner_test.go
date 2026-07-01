package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mework/libs/sandbox/agent"
	"mework/libs/shared/core"
)

// TestRunFeedsPromptViaStdin verifies the prompt reaches the backend on stdin
// (not as a shell arg) and stdout is captured. Uses `cat` as a stand-in CLI.
func TestRunFeedsPromptViaStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix cat")
	}
	catPath := "/bin/cat"
	if _, err := os.Stat(catPath); err != nil {
		t.Skip("cat not available")
	}
	b := agent.Backend{Name: "cat", Path: catPath}
	work := filepath.Join(t.TempDir(), "ticket1")
	res := Run(context.Background(), b, "hello from ticket", work, 5*time.Second)
	if res.Err != nil {
		t.Fatalf("run failed: %v", res.Err)
	}
	if !strings.Contains(res.Output, "hello from ticket") {
		t.Errorf("stdin prompt not echoed to output: %q", res.Output)
	}
	if _, err := os.Stat(work); err != nil {
		t.Errorf("work dir not created: %v", err)
	}
}

// TestStart_WorkspaceBoundWorkingDir verifies the local engine's working
// directory selection: when a workspace is bound (spec.Workspace.Path set), the
// agent runs in that directory (a file the agent writes lands there). When no
// workspace is bound, the working directory is derived from SandboxID as today.
// Realises delta-spec scenarios "Agent works in the bound workspace" and
// "Unbound run is unchanged".
func TestStart_WorkspaceBoundWorkingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sh")
	}
	sh := "/bin/sh"
	if _, err := os.Stat(sh); err != nil {
		t.Skip("sh not available")
	}

	tests := []struct {
		name string
		// buildSpec returns the RunSpec and the directory the agent's relative
		// write is expected to land in.
		buildSpec func(t *testing.T) (core.RunSpec, string)
	}{
		{
			name: "workspace bound: file lands in the bound workspace dir",
			buildSpec: func(t *testing.T) (core.RunSpec, string) {
				t.Helper()
				ws := t.TempDir()
				return core.RunSpec{
					AgentID:   "agent-ws",
					SandboxID: "sandbox-should-be-ignored",
					Workspace: core.Workspace{ID: "w1", Path: ws},
				}, ws
			},
		},
		{
			name: "workspace unbound: workdir derived from SandboxID (unchanged)",
			buildSpec: func(t *testing.T) (core.RunSpec, string) {
				t.Helper()
				id := filepath.Join(t.TempDir(), "sandbox-id-dir")
				return core.RunSpec{
					AgentID:   "agent-nows",
					SandboxID: id,
				}, id
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, wantDir := tt.buildSpec(t)
			drv := New()
			sb, err := drv.Start(context.Background(), spec)
			if err != nil {
				t.Fatalf("Start: %v", err)
			}

			// The agent writes a marker file using a RELATIVE path, so where it
			// lands reveals the sandbox working directory.
			const marker = "artifact.txt"
			var out bytes.Buffer
			exit, execErr := sb.Exec(
				context.Background(),
				[]string{sh, "-c", "printf agent-output > " + marker},
				strings.NewReader(""),
				&out, &out,
			)
			if execErr != nil || exit != 0 {
				t.Fatalf("exec failed: exit=%d err=%v out=%q", exit, execErr, out.String())
			}

			got, err := os.ReadFile(filepath.Join(wantDir, marker))
			if err != nil {
				t.Fatalf("expected artifact in %q: %v", wantDir, err)
			}
			if string(got) != "agent-output" {
				t.Errorf("artifact content = %q, want %q", got, "agent-output")
			}
		})
	}
}

func TestRunNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/false")
	}
	if _, err := os.Stat("/usr/bin/false"); err != nil {
		if _, err := os.Stat("/bin/false"); err != nil {
			t.Skip("false not available")
		}
	}
	path := "/usr/bin/false"
	if _, err := os.Stat(path); err != nil {
		path = "/bin/false"
	}
	res := Run(context.Background(), agent.Backend{Name: "false", Path: path}, "x", t.TempDir(), 5*time.Second)
	if res.Err == nil {
		t.Error("expected error from non-zero exit")
	}
	if res.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}
