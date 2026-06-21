package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mework/libs/sandbox/agent"
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
