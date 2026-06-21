package runner

import (
	"os"
	"testing"

	"mework/libs/shared/config"
)

func TestHealthPortDeterministic(t *testing.T) {
	a := HealthPort("dev")
	b := HealthPort("dev")
	if a != b {
		t.Errorf("same profile gave different ports: %d != %d", a, b)
	}
	if HealthPort("dev") == HealthPort("prod") {
		t.Error("different profiles should (almost always) map to different ports")
	}
	if a < healthBasePort || a >= healthBasePort+1000 {
		t.Errorf("port %d out of expected range", a)
	}
}

func TestPIDLifecycle(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	const prof = "test"

	if running, _ := IsRunning(prof); running {
		t.Fatal("should not be running before WritePID")
	}
	if err := WritePID(prof); err != nil {
		t.Fatal(err)
	}
	// The current process wrote its own pid, so it must look alive.
	running, pid := IsRunning(prof)
	if !running || pid != os.Getpid() {
		t.Fatalf("expected running with our pid, got running=%v pid=%d", running, pid)
	}
	if err := RemovePID(prof); err != nil {
		t.Fatal(err)
	}
	if running, _ := IsRunning(prof); running {
		t.Error("should not be running after RemovePID")
	}
}

func TestIsRunningStalePID(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	const prof = "stale"
	// Write a pid that is extremely unlikely to be a live process.
	if err := WritePID(prof); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.PidPath(prof), []byte("999999"), 0o600); err != nil {
		t.Fatal(err)
	}
	if running, _ := IsRunning(prof); running {
		t.Error("a stale pid should not report running")
	}
}
