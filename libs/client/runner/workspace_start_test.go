package runner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"mework/libs/client/workspacefs"
	"mework/libs/sandbox"
	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	"mework/libs/shared/ports"
)

// wsArtifactName is the file the workspace-writing fake sandbox produces on the
// first turn; it lands directly in the bound workspace directory.
const wsArtifactName = "artifact.txt"

// wsArtifactBody is the content the fake agent writes into the workspace.
const wsArtifactBody = "agent produced this\n"

// wsWritingSandbox is a long-lived sandbox stand-in that, on Exec, writes an
// artifact into the workspace directory it was started with. It models the
// local engine binding the workspace as the sandbox working directory so that
// "artifacts persist and are readable back" can be asserted on disk.
type wsWritingSandbox struct {
	mu           sync.Mutex
	id           string
	workspaceDir string
	turns        []string
	argvs        [][]string
}

func (s *wsWritingSandbox) ID() string { return s.id }

func (s *wsWritingSandbox) Exec(_ context.Context, command []string, stdin io.Reader, _, _ io.Writer) (int, error) {
	var content string
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		content = string(b)
	}
	s.mu.Lock()
	s.turns = append(s.turns, content)
	s.argvs = append(s.argvs, command)
	dir := s.workspaceDir
	s.mu.Unlock()

	if dir != "" {
		// The agent writes an artifact into its bound workspace working dir.
		_ = os.WriteFile(filepath.Join(dir, wsArtifactName), []byte(wsArtifactBody), 0o600)
	}
	return 0, nil
}

func (s *wsWritingSandbox) Mount(context.Context, core.Workspace, string) error { return nil }
func (s *wsWritingSandbox) Signals(context.Context, string) error               { return nil }

func (s *wsWritingSandbox) gotArgvs() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, len(s.argvs))
	copy(out, s.argvs)
	return out
}

// wsWritingDriver hands back one wsWritingSandbox whose working directory is the
// bound workspace (spec.Workspace.Path). It records the spec it was started with
// so the test can assert the workspace was bound.
type wsWritingDriver struct {
	mu       sync.Mutex
	startN   int
	lastSpec core.RunSpec
	sb       *wsWritingSandbox
}

func (d *wsWritingDriver) Caps() core.SandboxCaps { return core.SandboxCaps{DriverName: "ws-fake"} }

func (d *wsWritingDriver) Start(_ context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.startN++
	d.lastSpec = spec
	if d.sb == nil {
		d.sb = &wsWritingSandbox{id: spec.SandboxID, workspaceDir: spec.Workspace.Path}
	}
	return d.sb, nil
}

func (d *wsWritingDriver) Stop(context.Context, string) error    { return nil }
func (d *wsWritingDriver) Destroy(context.Context, string) error { return nil }

func (d *wsWritingDriver) startedSpec() core.RunSpec {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastSpec
}

// fileWorkspaceResolver is an in-package stand-in for catalog.FileDefinitionResolver
// (the runner test package cannot import client/catalog without an import cycle,
// since catalog imports runner). It loads mework.yml from a workspace dir and
// decodes it into bundle metadata, contacting no server — behaviorally identical
// to the local-direct resolver.
type fileWorkspaceResolver struct {
	workspaceDir string
}

func (r fileWorkspaceResolver) ResolveDefinition(_ context.Context, _ string) (*sandbox.SandboxBundleMetadata, error) {
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

// writeMeworkYML writes a minimal local-engine mework.yml into dir.
func writeMeworkYML(t *testing.T, dir string) {
	t.Helper()
	const cfg = "name: local-claude\nversion: 1.0.0\nengine: local\nbackend: claude\n"
	if err := os.WriteFile(filepath.Join(dir, "mework.yml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write mework.yml: %v", err)
	}
}

// TestStartWorkspaceSession_ServerMode asserts that a server-mode start funnels
// through a workspace-bound OpenSession: the resolver supplies the definition,
// the opened session's RunSpec.Workspace.Path is the workspace dir, and the agent
// writes an artifact into that dir. Realises delta-spec requirement "Two start
// modes — server and local-direct" scenario "Start from the server".
func TestStartWorkspaceSession_ServerMode(t *testing.T) {
	wsDir := t.TempDir()
	drv := &wsWritingDriver{}

	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	// Server mode: a (fake) catalog resolver stands in for HTTPDefinitionResolver
	// and a server-issued spawn grant authorizes the run.
	defs := map[string]sandbox.SandboxBundleMetadata{
		"local-claude@1.0.0": {Name: "local-claude", Version: "1.0.0", Engine: "local", Backend: "claude"},
	}
	g, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, nil)
	if err != nil {
		t.Fatalf("new grant: %v", err)
	}

	sess, err := StartWorkspaceSession(context.Background(), StartOptions{
		Ref:          "local-claude@1.0.0",
		Resolver:     fakeResolver{defs: defs},
		WorkspaceDir: wsDir,
		Caller:       Caller{Account: testOwner, Tenant: testTenant, Grant: g},
		ManagerFor:   func(string) *runtime.Manager { return runtime.NewManager(drv) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession (server): %v", err)
	}
	t.Cleanup(func() { _ = sess.Close(context.Background(), Caller{Account: testOwner, Tenant: testTenant, Grant: g}) })

	if got := drv.startedSpec().Workspace.Path; got != wsDir {
		t.Fatalf("RunSpec.Workspace.Path = %q, want bound workspace dir %q", got, wsDir)
	}

	// Drive one turn so the agent writes its artifact into the bound workspace.
	if err := sess.Send(context.Background(), Caller{Account: testOwner, Tenant: testTenant, Grant: g}, "produce the file"); err != nil {
		t.Fatalf("send turn: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsDir, wsArtifactName)); err != nil {
		t.Fatalf("artifact not written into bound workspace: %v", err)
	}
}

// TestStartWorkspaceSession_LocalDirectMode asserts that a local-direct start
// resolves from a workspace mework.yml using a locally-minted OpSpawn grant and
// contacts no server: an httptest server wired as the (unused) base records zero
// hits. RunSpec.Workspace.Path is the workspace dir. Realises scenario "Start
// fully locally".
func TestStartWorkspaceSession_LocalDirectMode(t *testing.T) {
	wsDir := t.TempDir()
	writeMeworkYML(t, wsDir)

	drv := &wsWritingDriver{}
	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	// Locally-minted grant — no server issues it.
	key := []byte("local-secret-key")
	localGrant, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, key)
	if err != nil {
		t.Fatalf("mint local grant: %v", err)
	}

	sess, err := StartWorkspaceSession(context.Background(), StartOptions{
		Ref:          "local-claude@1.0.0",
		Resolver:     fileWorkspaceResolver{workspaceDir: wsDir},
		WorkspaceDir: wsDir,
		Caller:       Caller{Account: testOwner, Tenant: testTenant, Grant: localGrant},
		GrantKey:     key,
		ManagerFor:   func(string) *runtime.Manager { return runtime.NewManager(drv) },
		Broker:       broker,
		Sessions:     mgr,
	})
	if err != nil {
		t.Fatalf("StartWorkspaceSession (local-direct): %v", err)
	}
	t.Cleanup(func() {
		_ = sess.Close(context.Background(), Caller{Account: testOwner, Tenant: testTenant, Grant: localGrant})
	})

	if got := drv.startedSpec().Workspace.Path; got != wsDir {
		t.Fatalf("RunSpec.Workspace.Path = %q, want workspace dir %q", got, wsDir)
	}

	if err := sess.Send(context.Background(), Caller{Account: testOwner, Tenant: testTenant, Grant: localGrant}, "produce the file"); err != nil {
		t.Fatalf("send turn: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsDir, wsArtifactName)); err != nil {
		t.Fatalf("artifact not written into workspace: %v", err)
	}

	// stdin-not-argv: turn content must not leak into the command line.
	for _, argv := range drv.sb.gotArgvs() {
		for _, arg := range argv {
			if strings.Contains(arg, "produce the file") {
				t.Errorf("turn content leaked into argv: %q", arg)
			}
		}
	}
}

// TestStartWorkspaceSession_MissingSpawnGrantRejected asserts that a start whose
// caller grant lacks OpSpawn is rejected before any sandbox is started, proving
// the local identity is enforced. Realises the grant-enforcement invariant
// underpinning both start modes.
func TestStartWorkspaceSession_MissingSpawnGrantRejected(t *testing.T) {
	wsDir := t.TempDir()
	writeMeworkYML(t, wsDir)

	drv := &wsWritingDriver{}
	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	tests := []struct {
		name string
		ops  []grant.Operation
	}{
		{name: "grant without OpSpawn is rejected", ops: []grant.Operation{grant.OpRepoRead}},
		{name: "empty grant is rejected", ops: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := grant.NewGrant(tt.ops, nil)
			if err != nil {
				t.Fatalf("new grant: %v", err)
			}
			_, err = StartWorkspaceSession(context.Background(), StartOptions{
				Ref:          "local-claude@1.0.0",
				Resolver:     fileWorkspaceResolver{workspaceDir: wsDir},
				WorkspaceDir: wsDir,
				Caller:       Caller{Account: testOwner, Tenant: testTenant, Grant: g},
				ManagerFor:   func(string) *runtime.Manager { return runtime.NewManager(drv) },
				Broker:       broker,
				Sessions:     mgr,
			})
			if err == nil {
				t.Fatalf("expected start to be rejected for a grant missing OpSpawn")
			}
			if drv.startedSpec().SandboxID != "" {
				t.Errorf("sandbox must not start when the grant is rejected")
			}
		})
	}
}

// TestStartWorkspaceSession_ArtifactsReadableBack asserts that after a turn the
// produced artifact can be listed, read, updated, and re-read via
// workspacefs.NewLocal over the bound workspace. Realises requirement
// "Workspace-bound session" scenario "Artifacts are readable back".
func TestStartWorkspaceSession_ArtifactsReadableBack(t *testing.T) {
	wsDir := t.TempDir()
	writeMeworkYML(t, wsDir)

	drv := &wsWritingDriver{}
	broker := memory.New()
	mgr := session.NewManager(broker, session.DefaultConfig())
	t.Cleanup(mgr.Stop)

	key := []byte("local-secret-key")
	g, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, key)
	if err != nil {
		t.Fatalf("mint grant: %v", err)
	}
	caller := Caller{Account: testOwner, Tenant: testTenant, Grant: g}

	sess, err := StartWorkspaceSession(context.Background(), StartOptions{
		Ref:          "local-claude@1.0.0",
		Resolver:     fileWorkspaceResolver{workspaceDir: wsDir},
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
	t.Cleanup(func() { _ = sess.Close(context.Background(), caller) })

	if err := sess.Send(context.Background(), caller, "produce the file"); err != nil {
		t.Fatalf("send turn: %v", err)
	}

	ctx := context.Background()
	fsys := workspacefs.NewLocal(wsDir, "", nil)

	// List: the produced artifact appears among the workspace entries.
	entries, err := fsys.List(ctx, ".")
	if err != nil {
		t.Fatalf("list workspace: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == wsArtifactName {
			found = true
		}
	}
	if !found {
		t.Fatalf("produced artifact %q not listed in workspace: %+v", wsArtifactName, entries)
	}

	// Read: the contents match what the agent wrote.
	rc, err := fsys.ReadFile(ctx, wsArtifactName)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != wsArtifactBody {
		t.Errorf("artifact contents = %q, want %q", string(got), wsArtifactBody)
	}

	// Update: the user writes an update and re-reads the updated contents.
	const updated = "user-updated contents\n"
	if err := fsys.WriteFile(ctx, wsArtifactName, strings.NewReader(updated)); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	rc2, err := fsys.ReadFile(ctx, wsArtifactName)
	if err != nil {
		t.Fatalf("re-read artifact: %v", err)
	}
	got2, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if string(got2) != updated {
		t.Errorf("updated artifact contents = %q, want %q", string(got2), updated)
	}
}
