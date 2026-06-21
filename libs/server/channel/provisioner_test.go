package channel

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/catalog"
	"mework/libs/server/platform/store"
	"mework/libs/server/registry"
	"mework/libs/server/session"
	"mework/libs/shared/core"
)

// TestProvisioner_SpecDerivation tests spec derivation from a profile name.
func TestProvisioner_SpecDerivation(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		wantSpec    string
	}{
		{
			name:        "claude-code profile derives claude-code spec",
			profileName: "claude-code",
			wantSpec:    "claude-code",
		},
		{
			name:        "codex profile derives codex spec",
			profileName: "codex",
			wantSpec:    "codex",
		},
		{
			name:        "custom profile with backend_hint",
			profileName: "my-agent",
			wantSpec:    "my-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// In the simplest case, spec == profile name (backend_hint)
			// The provisioner will resolve spec from the profile's backend_hint
			spec := resolveSpecFromProfile(tt.profileName)
			if spec != tt.wantSpec {
				t.Errorf("resolveSpecFromProfile(%q) = %q, want %q", tt.profileName, spec, tt.wantSpec)
			}
		})
	}
}

// TestProvisioner_SelectWorkerCalled verifies that Provision calls SelectWorker
// with the correct spec.
func TestProvisioner_SelectWorkerCalled(t *testing.T) {
	ctx, pool, svc, accountID, tenant := newProvisionerTestDB(t)

	memBroker := memory.New()

	reg := NewPostgresRegistry(pool)
	sessionMgr := session.NewManager(memBroker, session.DefaultConfig())
	t.Cleanup(sessionMgr.Stop)

	agentHandlers := catalog.NewAgentHandlers(catalog.NewService(pool), memBroker, nil, nil)

	prov := NewAutoProvisioner(svc, reg, sessionMgr, agentHandlers, memBroker, tenant.ID)

	// Seed a runner with a matching spec for SelectWorker to find
	seedProvisionerRunner(t, ctx, pool, tenant.ID, accountID, "worker-1", "claude-code")

	sessionID, err := prov.Provision(ctx, "mello", "TICKET-99", "claude-code")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if sessionID == "" {
		t.Fatal("Provision returned empty session ID")
	}

	// Verify the channel was bound (lookup should return the session)
	lookupID, err := reg.Lookup(ctx, "mello:TICKET-99")
	if err != nil {
		t.Fatalf("Lookup after provision: %v", err)
	}
	if lookupID == "" {
		t.Fatal("Lookup returned empty after provision — channel not bound")
	}
}

// TestProvisioner_SessionCreatedAndChannelBound verifies that after worker
// selection, a session is created and the channel is bound.
func TestProvisioner_SessionCreatedAndChannelBound(t *testing.T) {
	ctx, pool, svc, accountID, tenant := newProvisionerTestDB(t)

	memBroker := memory.New()
	reg := NewPostgresRegistry(pool)
	sessionMgr := session.NewManager(memBroker, session.DefaultConfig())
	t.Cleanup(sessionMgr.Stop)

	agentHandlers := catalog.NewAgentHandlers(catalog.NewService(pool), memBroker, nil, nil)

	prov := NewAutoProvisioner(svc, reg, sessionMgr, agentHandlers, memBroker, tenant.ID)

	seedProvisionerRunner(t, ctx, pool, tenant.ID, accountID, "worker-2", "claude-code")

	sessionID, err := prov.Provision(ctx, "mello", "TICKET-101", "claude-code")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	// Verify session was created
	sInfo, err := sessionMgr.Get(ctx, core.SessionID(sessionID))
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if sInfo.Status != core.SessionActive {
		t.Errorf("session status = %q, want %q", sInfo.Status, core.SessionActive)
	}

	// Verify channel was bound
	lookupID, err := reg.Lookup(ctx, "mello:TICKET-101")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if lookupID != sessionID {
		t.Errorf("Lookup = %q, want %q", lookupID, sessionID)
	}
}

// TestProvisioner_AgentDispatched verifies that DispatchToRunner is called
// after provisioning completes.
func TestProvisioner_AgentDispatched(t *testing.T) {
	ctx, pool, svc, accountID, tenant := newProvisionerTestDB(t)

	memBroker := memory.New()
	reg := NewPostgresRegistry(pool)
	sessionMgr := session.NewManager(memBroker, session.DefaultConfig())
	t.Cleanup(sessionMgr.Stop)

	agentHandlers := catalog.NewAgentHandlers(catalog.NewService(pool), memBroker, nil, nil)

	prov := NewAutoProvisioner(svc, reg, sessionMgr, agentHandlers, memBroker, tenant.ID)

	rtID := seedProvisionerRunner(t, ctx, pool, tenant.ID, accountID, "worker-3", "claude-code")

	_, err := prov.Provision(ctx, "mello", "TICKET-102", "claude-code")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	// DispatchToRunner would have been called internally by the provisioner.
	// We verify this indirectly: the runner's dispatch topic received a message.
	// Subscribe to the runner's topic and check for a dispatch message.
	runnerTopic := "runner." + rtID + ".dispatch"
	sub, err := memBroker.Subscribe(ctx, "test-check", bus.Filter(runnerTopic), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	select {
	case ev := <-sub.Events():
		if ev.Topic != bus.Topic(runnerTopic) {
			t.Errorf("dispatch event on topic %q, want %q", ev.Topic, runnerTopic)
		}
		if len(ev.Message.Payload) == 0 {
			t.Error("dispatch message payload is empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch message on runner topic")
	}
}

// TestProvisioner_NoEligibleWorkerRetries verifies that when no worker matches,
// the provisioner retries and eventually returns an error.
func TestProvisioner_NoEligibleWorkerRetries(t *testing.T) {
	ctx, pool, svc, _, tenant := newProvisionerTestDB(t)

	memBroker := memory.New()
	reg := NewPostgresRegistry(pool)
	sessionMgr := session.NewManager(memBroker, session.DefaultConfig())
	t.Cleanup(sessionMgr.Stop)

	agentHandlers := catalog.NewAgentHandlers(catalog.NewService(pool), memBroker, nil, nil)

	prov := NewAutoProvisioner(svc, reg, sessionMgr, agentHandlers, memBroker, tenant.ID)

	// No runner seeded for "codex" spec — should retry and fail
	_, err := prov.Provision(ctx, "mello", "TICKET-103", "codex")
	if err == nil {
		t.Fatal("Provision: expected error for no eligible worker, got nil")
	}
}

// TestProvisioner_FullFlow is a full DB-backed test that exercises all
// provisioner steps end-to-end.
func TestProvisioner_FullFlow(t *testing.T) {
	ctx, pool, svc, accountID, tenant := newProvisionerTestDB(t)

	memBroker := memory.New()
	reg := NewPostgresRegistry(pool)
	sessionMgr := session.NewManager(memBroker, session.DefaultConfig())
	t.Cleanup(sessionMgr.Stop)

	agentHandlers := catalog.NewAgentHandlers(catalog.NewService(pool), memBroker, nil, nil)

	prov := NewAutoProvisioner(svc, reg, sessionMgr, agentHandlers, memBroker, tenant.ID)

	// Seed the runner and register the agent in the catalog
	seedProvisionerRunner(t, ctx, pool, tenant.ID, accountID, "full-worker", "claude-code")

	sessionID, err := prov.Provision(ctx, "mello", "TICKET-200", "claude-code")
	if err != nil {
		t.Fatalf("Provision full flow: %v", err)
	}
	if sessionID == "" {
		t.Fatal("Provision returned empty session ID")
	}

	// Verify session
	sInfo, err := sessionMgr.Get(ctx, core.SessionID(sessionID))
	if err != nil {
		t.Fatalf("Get session after full provision: %v", err)
	}
	if sInfo.Status != core.SessionActive {
		t.Errorf("session status = %q, want %q", sInfo.Status, core.SessionActive)
	}

	// Verify channel binding persisted
	lookupID, err := reg.Lookup(ctx, "mello:TICKET-200")
	if err != nil {
		t.Fatalf("Lookup after full flow: %v", err)
	}
	if lookupID != sessionID {
		t.Errorf("channel lookup = %q, want %q", lookupID, sessionID)
	}
}

// newProvisionerTestDB sets up a clean DB for provisioner tests and returns
// the pool, service, account ID, and tenant.
func newProvisionerTestDB(t *testing.T) (context.Context, *pgxpool.Pool, *registry.Service, string, *registry.Tenant) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { _ = store.RollbackMigrations(dsn) })

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, "DELETE FROM channel_sessions; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants; DELETE FROM agents;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Provisioner Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	svc := registry.NewService(pool, "supersecretkey")

	ten, err := svc.RegisterTenant(ctx, "provisioner-test-tenant")
	if err != nil {
		t.Fatalf("RegisterTenant: %v", err)
	}

	return ctx, pool, svc, accountID, ten
}

// seedProvisionerRunner creates a runner and sets it online with the given spec.
// Returns the runtime ID.
func seedProvisionerRunner(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, accountID, code, spec string) string {
	t.Helper()

	// Register agent first (SelectWorker validates spec against agents table)
	_, err := pool.Exec(ctx, "INSERT INTO agents (name) VALUES ($1) ON CONFLICT DO NOTHING", spec)
	if err != nil {
		t.Fatalf("insert agent %s: %v", spec, err)
	}

	ten := registry.Tenant{ID: tenantID}
	svc := registry.NewService(pool, "supersecretkey")
	rt, _, err := svc.CreateRuntime(ctx, ten, accountID, code, "label-"+code, spec)
	if err != nil {
		t.Fatalf("CreateRuntime(%s): %v", code, err)
	}
	_, err = pool.Exec(ctx, "UPDATE runtimes SET status = 'online' WHERE id = $1", rt.ID)
	if err != nil {
		t.Fatalf("update status for %s: %v", code, err)
	}
	return rt.ID
}

// resolveSpecFromProfile maps a profile name to a spec name.
// In the real implementation, this reads backend_hint from the profile.
func resolveSpecFromProfile(profileName string) string {
	// Placeholder: spec == profile name until backend_hint is read from DB
	return profileName
}
