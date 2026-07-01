package catalog

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

func testDSN() string {
	return os.Getenv("TEST_DATABASE_URL")
}

// setupAgentTestDB runs all migrations and returns a clean pool with common
// tables truncated. It does NOT truncate agent tables so the ProfilesMigration
// test can control agent-table state separately.
func setupAgentTestDB(t *testing.T, dsn string) (*pgxpool.Pool, func()) {
	t.Helper()

	ctx := context.Background()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	// Clean FK-safe: delete dependent tables first, then the tables migrations
	// expect to be clean for each test case.
	if _, err := pool.Exec(ctx, `
		DELETE FROM watched_containers;
		DELETE FROM account_identities;
		DELETE FROM profiles;
		DELETE FROM accounts;
	`); err != nil {
		pool.Close()
		t.Fatalf("failed to clean db: %v", err)
	}

	cleanup := func() {
		pool.Close()
		_ = store.RollbackMigrations(dsn)
	}

	return pool, cleanup
}

func TestAgentStore_CreateVersion(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	agent, err := svc.CreateAgent(ctx, "code-fixer", "Fixes code issues")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if agent.Name != "code-fixer" {
		t.Errorf("agent.Name = %q, want %q", agent.Name, "code-fixer")
	}
	if agent.ID == "" {
		t.Error("expected non-empty agent ID")
	}

	payload := []byte(`{"prompt": "fix the code"}`)
	v, err := svc.PublishVersion(ctx, agent.ID, "1.2.0", "definition", payload, "")
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}

	if v.Version != "1.2.0" {
		t.Errorf("v.Version = %q, want %q", v.Version, "1.2.0")
	}
	if v.Form != "definition" {
		t.Errorf("v.Form = %q, want %q", v.Form, "definition")
	}
	if v.AgentID != agent.ID {
		t.Errorf("v.AgentID = %q, want %q", v.AgentID, agent.ID)
	}
	if v.ID == "" {
		t.Error("expected non-empty version ID")
	}
	if v.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if v.Checksum == "" {
		t.Error("expected non-empty checksum")
	}

	// Verify retrievable by exact lookup via Resolve
	got, err := svc.Resolve(ctx, "code-fixer", "1.2.0")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != v.ID {
		t.Errorf("Resolve returned ID %q, want %q", got.ID, v.ID)
	}
}

func TestAgentStore_CreateVersionDuplicate(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	agent, err := svc.CreateAgent(ctx, "code-fixer", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	payload := []byte(`{"prompt": "fix the code"}`)
	if _, err := svc.PublishVersion(ctx, agent.ID, "1.0.0", "definition", payload, ""); err != nil {
		t.Fatalf("first publish should succeed: %v", err)
	}

	_, err = svc.PublishVersion(ctx, agent.ID, "1.0.0", "definition", payload, "")
	if err != ErrVersionAlreadyExists {
		t.Errorf("expected ErrVersionAlreadyExists, got %v", err)
	}
}

func TestAgentStore_ResolveLatest(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	agent, err := svc.CreateAgent(ctx, "code-fixer", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	payload := []byte(`{"v": 1}`)
	v1, err := svc.PublishVersion(ctx, agent.ID, "1.0.0", "definition", payload, "")
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	v2, err := svc.PublishVersion(ctx, agent.ID, "2.0.0", "definition", payload, "")
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}

	// latest should point at v2 (most recently published)
	got, err := svc.Resolve(ctx, "code-fixer", "latest")
	if err != nil {
		t.Fatalf("Resolve latest: %v", err)
	}
	if got.ID != v2.ID {
		t.Errorf("Resolve returned version %q (%s), want %q", got.Version, got.ID, v2.ID)
	}
	_ = v1
}

func TestAgentStore_ResolveNamedChannel(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	agent, err := svc.CreateAgent(ctx, "code-fixer", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	payload := []byte(`{"v": 1}`)
	v1, err := svc.PublishVersion(ctx, agent.ID, "1.0.0", "definition", payload, "")
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	_, err = svc.PublishVersion(ctx, agent.ID, "2.0.0", "definition", payload, "")
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}

	// Set channel "stable" to point at v1
	if err := svc.SetChannelPointer(ctx, agent.ID, "stable", v1.ID); err != nil {
		t.Fatalf("SetChannelPointer: %v", err)
	}

	got, err := svc.Resolve(ctx, "code-fixer", "stable")
	if err != nil {
		t.Fatalf("Resolve stable: %v", err)
	}
	if got.ID != v1.ID {
		t.Errorf("Resolve returned version %q (%s), want %q", got.Version, got.ID, v1.ID)
	}
}

func TestAgentStore_ResolveExactVersion(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	agent, err := svc.CreateAgent(ctx, "code-fixer", "")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	payload := []byte(`{"v": 1}`)
	v1, err := svc.PublishVersion(ctx, agent.ID, "1.0.0", "definition", payload, "")
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	// Resolve exact version that exists
	got, err := svc.Resolve(ctx, "code-fixer", "1.0.0")
	if err != nil {
		t.Fatalf("Resolve exact: %v", err)
	}
	if got.ID != v1.ID {
		t.Errorf("Resolve returned ID %q, want %q", got.ID, v1.ID)
	}

	// Resolve version that does not exist
	_, err = svc.Resolve(ctx, "code-fixer", "9.9.9")
	if err != ErrNotFound {
		t.Errorf("Resolve missing version: expected ErrNotFound, got %v", err)
	}
}

func TestAgentStore_ListAgents(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	if _, err := svc.CreateAgent(ctx, "agent-a", "Agent A"); err != nil {
		t.Fatalf("create agent-a: %v", err)
	}
	if _, err := svc.CreateAgent(ctx, "agent-b", "Agent B"); err != nil {
		t.Fatalf("create agent-b: %v", err)
	}

	agents, err := svc.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("len(agents) = %d, want 2", len(agents))
	}

	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name] = true
	}
	if !names["agent-a"] {
		t.Error("expected agent-a in list")
	}
	if !names["agent-b"] {
		t.Error("expected agent-b in list")
	}
}

func TestAgentStore_ProfilesMigration(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	ctx := context.Background()

	// Ensure a default account exists so profile FKs work.
	var accountID string
	if err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('default') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}

	// Seed profiles that the 000004 migration should copy to agents.
	if _, err := pool.Exec(ctx, `INSERT INTO profiles (id, account_id, name, body) VALUES ($1, $2, $3, $4)`,
		"00000000-0000-0000-0000-000000000001", accountID, "dev", "system prompt for dev",
	); err != nil {
		t.Fatalf("insert profile dev: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO profiles (id, account_id, name, body) VALUES ($1, $2, $3, $4)`,
		"00000000-0000-0000-0000-000000000002", accountID, "prod", "system prompt for prod",
	); err != nil {
		t.Fatalf("insert profile prod: %v", err)
	}

	// Run the profiles→agents data-migration queries that 000004_agent_catalog.sql
	// should include.  Each profile row becomes a definition-form agent with version 1.0.0.
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, name, description, created_at)
		SELECT gen_random_uuid(), p.name, '', NOW()
		FROM profiles p
		WHERE NOT EXISTS (SELECT 1 FROM agents a WHERE a.name = p.name)
	`); err != nil {
		t.Fatalf("data migration (agents insert): %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_versions (id, agent_id, version, form, payload, checksum, created_at)
		SELECT gen_random_uuid(), a.id, '1.0.0', 'definition', p.body::bytea, md5(p.body), NOW()
		FROM profiles p
		JOIN agents a ON a.name = p.name
		WHERE NOT EXISTS (
			SELECT 1 FROM agent_versions av WHERE av.agent_id = a.id AND av.version = '1.0.0'
		)
	`); err != nil {
		t.Fatalf("data migration (agent_versions insert): %v", err)
	}

	// Validate the migration result via the store methods.
	svc := NewService(pool)

	agents, err := svc.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents after migration: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents after migration, got %d", len(agents))
	}

	for _, name := range []string{"dev", "prod"} {
		v, err := svc.Resolve(ctx, name, "latest")
		if err != nil {
			t.Fatalf("Resolve %s@latest: %v", name, err)
		}
		if v.Form != "definition" {
			t.Errorf("%s: expected form 'definition', got %q", name, v.Form)
		}
		if v.Version != "1.0.0" {
			t.Errorf("%s: expected version '1.0.0', got %q", name, v.Version)
		}
	}
}
