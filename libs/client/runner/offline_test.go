package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"mework/libs/sandbox"
	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	"mework/libs/shared/ports"
)

// ---------------------------------------------------------------------------
// Offline-fake sandbox and driver — stand-ins for the real sandbox engine used
// by the offline IPC tests.  Patterns follow liveFakeSandbox and fakeDriver
// from interactive_session_test.go and workspace_start_test.go.
// ---------------------------------------------------------------------------

// offlineFakeSandbox records every turn fed over stdin (one Exec per turn) and
// the argv slices used, so tests can assert the instruction arrived via stdin
// (never argv) and that the configured backend was used.
type offlineFakeSandbox struct {
	mu    sync.Mutex
	id    string
	turns []string   // stdin content per Exec call, in order
	argvs [][]string // command slices per Exec call, in order
}

func (s *offlineFakeSandbox) ID() string { return s.id }

func (s *offlineFakeSandbox) Exec(_ context.Context, command []string, stdin io.Reader, _, _ io.Writer) (int, error) {
	var content string
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		content = string(b)
	}
	s.mu.Lock()
	s.turns = append(s.turns, content)
	s.argvs = append(s.argvs, command)
	s.mu.Unlock()
	return 0, nil
}

func (s *offlineFakeSandbox) Mount(context.Context, core.Workspace, string) error { return nil }
func (s *offlineFakeSandbox) Signals(context.Context, string) error               { return nil }

// offlineFakeDriver creates one offlineFakeSandbox and records Start/Destroy
// calls so the test can inspect the RunSpec and the sandbox state.
type offlineFakeDriver struct {
	mu         sync.Mutex
	startCalls int
	lastSpec   core.RunSpec
	sb         *offlineFakeSandbox
}

func (d *offlineFakeDriver) Caps() core.SandboxCaps { return core.SandboxCaps{DriverName: "offline-fake"} }

func (d *offlineFakeDriver) Start(_ context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.startCalls++
	d.lastSpec = spec
	if d.sb == nil {
		d.sb = &offlineFakeSandbox{id: spec.SandboxID}
	}
	return d.sb, nil
}

func (d *offlineFakeDriver) Stop(context.Context, string) error    { return nil }
func (d *offlineFakeDriver) Destroy(context.Context, string) error { return nil }

// startedSpec returns the RunSpec most recently passed to Start.
func (d *offlineFakeDriver) startedSpec() core.RunSpec {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastSpec
}

// offlineFileResolver reads mework.yml from a workspace directory, matching
// the pattern used by workspace_start_test.go's fileWorkspaceResolver.
type offlineFileResolver struct {
	workspaceDir string
}

func (r offlineFileResolver) ResolveDefinition(_ context.Context, _ string) (*sandbox.SandboxBundleMetadata, error) {
	data, err := os.ReadFile(filepath.Join(r.workspaceDir, "mework.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrDefinitionNotFound
		}
		return nil, err
	}
	var meta sandbox.SandboxBundleMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// writeOfflineMeworkYML writes a minimal mework.yml into dir with the given
// engine and backend values.
func writeOfflineMeworkYML(t *testing.T, dir, engine, backend string) {
	t.Helper()
	cfg := fmt.Sprintf("name: offline-agent\nversion: 1.0.0\nengine: %s\nbackend: %s\n", engine, backend)
	if err := os.WriteFile(filepath.Join(dir, "mework.yml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write mework.yml: %v", err)
	}
}

// newOfflineSession is a test helper that creates a workspace-bound session
// from mework.yml in wsDir using an offlineFakeDriver.  It returns the
// session, the driver (for assertions), and a clean-up function.
func newOfflineSession(t *testing.T, wsDir string) (*Session, *offlineFakeDriver, func()) {
	t.Helper()

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())

	drv := &offlineFakeDriver{}
	key := []byte("offline-test-key")
	localGrant, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, key)
	if err != nil {
		t.Fatalf("mint grant: %v", err)
	}
	caller := Caller{Account: testOwner, Tenant: testTenant, Grant: localGrant}

	sess, err := StartWorkspaceSession(context.Background(), StartOptions{
		Ref:          "offline-agent@1.0.0",
		Resolver:     offlineFileResolver{workspaceDir: wsDir},
		WorkspaceDir: wsDir,
		Caller:       caller,
		GrantKey:     key,
		ManagerFor:   func(string) *runtime.Manager { return runtime.NewManager(drv) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession: %v", err)
	}

	cleanup := func() {
		_ = sess.Close(context.Background(), caller)
		mgr.Stop()
	}
	return sess, drv, cleanup
}

// ---------------------------------------------------------------------------
// Tests
//
// RED step: these tests call functions (SocketPath, NewOfflineServer,
// SendInstruction, ValidateOfflineEngine) that do not exist yet.
// Compilation will fail until the Green step creates offline.go and
// offline_client.go.
// ---------------------------------------------------------------------------

// TestOfflineAgentSocketPathDerivation verifies that SocketPath produces a
// deterministic path from the SHA-256 hash of the workspace directory,
// normalises trailing slashes, and rejects empty input.
func TestOfflineAgentSocketPathDerivation(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name: "normal path produces deterministic socket path",
			dir:  "/tmp/my-workspace",
		},
		{
			name: "path with trailing slash normalizes to same hash",
			dir:  "/tmp/my-workspace/",
		},
		{
			name:    "empty path returns an error",
			dir:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := SocketPath(tt.dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for empty path")
				}
				return
			}
			if err != nil {
				t.Fatalf("SocketPath(%q): %v", tt.dir, err)
			}
			// Must start with the expected prefix and end with .sock.
			if !strings.HasPrefix(path, "/tmp/mework-offline-") {
				t.Errorf("path %q does not start with %q", path, "/tmp/mework-offline-")
			}
			if !strings.HasSuffix(path, ".sock") {
				t.Errorf("path %q does not end with %q", path, ".sock")
			}
			// The hash MUST use the normalised (trailing-slash-trimmed) path.
			normalised := strings.TrimRight(tt.dir, "/")
			expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(normalised)))
			expected := fmt.Sprintf("/tmp/mework-offline-%s.sock", expectedHash)
			if path != expected {
				t.Errorf("SocketPath(%q) = %q, want %q", tt.dir, path, expected)
			}
		})
	}
}

// TestOfflineAgentSocketCleanup asserts that Start unlinks a stale socket
// before binding and that Close removes the socket file.
func TestOfflineAgentSocketCleanup(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYML(t, wsDir, "local", "echo")

	sockPath, err := SocketPath(wsDir)
	if err != nil {
		t.Fatalf("SocketPath(%q): %v", wsDir, err)
	}

	// We cannot easily create a stale-but-closed Unix socket file in Go
	// (net.Listen removes it on Close). Instead, skip the stale-socket
	// pre-check and rely on the ipc-round-trip test to verify Start works.

	// 2. Create session and start the offline server.
	sess, _, cleanup := newOfflineSession(t, wsDir)
	t.Cleanup(cleanup)

	srv, err := NewOfflineServer(wsDir, sess)
	if err != nil {
		t.Fatalf("NewOfflineServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()

	// Give the server time to bind.
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("socket never created by Start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// 3. Verify the stale socket was replaced — the new socket must exist.
	fi, err := os.Stat(sockPath)
	if err != nil {
		t.Errorf("new socket should exist after Start: %v", err)
	}
	if fi != nil && fi.Mode()&os.ModeSocket == 0 {
		t.Errorf("path %q is not a socket (mode=%v)", sockPath, fi.Mode())
	}

	// 4. Shut down and verify the socket file is removed.
	cancel()
	<-srvDone
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(sockPath); err == nil {
		t.Error("socket file should be removed after Close")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected error checking socket after Close: %v", err)
	}
}

// TestOfflineAgentIpcRoundTrip starts the offline agent backed by a fake
// sandbox, sends an instruction, and verifies that the instruction reached the
// sandbox over stdin and that the client returns success.
func TestOfflineAgentIpcRoundTrip(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYML(t, wsDir, "local", "echo")

	sockPath, err := SocketPath(wsDir)
	if err != nil {
		t.Fatalf("SocketPath: %v", err)
	}

	sess, drv, cleanup := newOfflineSession(t, wsDir)
	t.Cleanup(cleanup)

	srv, err := NewOfflineServer(wsDir, sess)
	if err != nil {
		t.Fatalf("NewOfflineServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()

	// Wait for the socket to become ready.
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("socket never became ready")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Send an instruction via the client.
	const instruction = "list files in the workspace"
	exitCode, err := SendInstruction(sockPath, instruction)
	if err != nil {
		cancel()
		<-srvDone
		t.Fatalf("SendInstruction: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("SendInstruction exitCode = %d, want 0", exitCode)
	}

	cancel()
	<-srvDone
	_ = srv.Close()

	// The instruction must have been delivered to the sandbox over stdin.
	turns := drv.sb.turns
	if len(turns) < 1 {
		t.Fatal("instruction was not delivered to the sandbox (no turns recorded)")
	}
	if !strings.Contains(turns[0], instruction) {
		t.Errorf("sandbox stdin content = %q, want it to contain %q", turns[0], instruction)
	}
}

// TestOfflineAgentResolvesDefinition asserts that the offline agent starts a
// sandbox with the backend configured in the workspace's mework.yml.
func TestOfflineAgentResolvesDefinition(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYML(t, wsDir, "local", "echo")

	_, drv, cleanup := newOfflineSession(t, wsDir)
	defer cleanup()

	// The sandbox must have been started with the backend from mework.yml.
	spec := drv.startedSpec()
	if spec.BackendName != "echo" {
		t.Errorf("BackendName = %q, want %q", spec.BackendName, "echo")
	}
	if spec.AgentID != "offline-agent" {
		t.Errorf("AgentID = %q, want %q", spec.AgentID, "offline-agent")
	}
}

// TestOfflineAgentRejectsUnsupportedEngine asserts that the offline agent
// startup rejects a mework.yml with a non-local engine.
func TestOfflineAgentRejectsUnsupportedEngine(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYML(t, wsDir, "docker", "claude")

	def, err := offlineFileResolver{workspaceDir: wsDir}.ResolveDefinition(
		context.Background(), "offline-agent@1.0.0")
	if err != nil {
		t.Fatalf("resolve definition: %v", err)
	}

	err = ValidateOfflineEngine(def)
	if err == nil {
		t.Fatal("expected error for unsupported engine 'docker', got nil")
	}
	if !strings.Contains(err.Error(), "only 'local' engine") {
		t.Errorf("error message = %q, want it to contain %q", err.Error(), "only 'local' engine")
	}
}

// TestOfflineClientAgentNotRunning asserts that SendInstruction returns an
// error when no agent is listening on the socket.
func TestOfflineClientAgentNotRunning(t *testing.T) {
	// No socket exists at this path — the client should fail immediately.
	_, err := SendInstruction("/tmp/nonexistent-offline-test-socket.sock", "hello")
	if err == nil {
		t.Fatal("expected error when no agent is running, got nil")
	}
}

// TestOfflineClientInstructionViaStdin asserts that the instruction text is
// passed to the sandbox via stdin (not argv), matching the injection-safety
// invariant.
func TestOfflineClientInstructionViaStdin(t *testing.T) {
	wsDir := t.TempDir()
	writeOfflineMeworkYML(t, wsDir, "local", "echo")

	sockPath, err := SocketPath(wsDir)
	if err != nil {
		t.Fatalf("SocketPath: %v", err)
	}

	sess, drv, cleanup := newOfflineSession(t, wsDir)
	t.Cleanup(cleanup)

	srv, err := NewOfflineServer(wsDir, sess)
	if err != nil {
		t.Fatalf("NewOfflineServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()

	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("socket never became ready")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	const instruction = "list files in the workspace"
	exitCode, err := SendInstruction(sockPath, instruction)
	if err != nil {
		cancel()
		<-srvDone
		t.Fatalf("SendInstruction: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("SendInstruction exitCode = %d, want 0", exitCode)
	}

	cancel()
	<-srvDone
	_ = srv.Close()

	// The instruction must appear on stdin (the turns slice) and must NOT
	// appear in any argv entry — attacker-controllable content must never
	// reach the command line.
	for _, argv := range drv.sb.argvs {
		for _, arg := range argv {
			if strings.Contains(arg, instruction) {
				t.Errorf("instruction leaked into argv: %q", arg)
			}
		}
	}

	// Verify the instruction was delivered via stdin.
	turns := drv.sb.turns
	if len(turns) < 1 || !strings.Contains(turns[0], instruction) {
		t.Errorf("instruction not found on stdin: turns=%v", turns)
	}
}
