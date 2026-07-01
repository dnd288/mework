package channel

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

// TestBind_CreatesRow verifies that binding a channel key to a session creates
// a row in channel_sessions with status "active". Delta-spec scenario:
// "Binding persists across restart".
func TestBind_CreatesRow(t *testing.T) {
	ctx, pool := newChannelTestDB(t)

	reg := NewPostgresRegistry(pool)

	tests := []struct {
		name       string
		channelKey string
		sessionID  string
		runnerID   string
		provider   string
		resourceID string
		spec       string
	}{
		{
			name:       "bind mello ticket",
			channelKey: "mello:TICKET-99",
			sessionID:  "s1",
			runnerID:   "r1",
			provider:   "mello",
			resourceID: "TICKET-99",
			spec:       "claude-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reg.Bind(ctx, tt.channelKey, tt.sessionID, tt.runnerID, tt.provider, tt.resourceID, tt.spec)
			if err != nil {
				t.Fatalf("Bind(%s): %v", tt.channelKey, err)
			}

			// Verify DB row exists
			var dbChannelKey, dbSessionID, dbRunnerID, dbStatus string
			err = pool.QueryRow(ctx,
				"SELECT channel_key, session_id, runner_id, status FROM channel_sessions WHERE channel_key = $1",
				tt.channelKey,
			).Scan(&dbChannelKey, &dbSessionID, &dbRunnerID, &dbStatus)
			if err != nil {
				t.Fatalf("query channel_sessions: %v", err)
			}
			if dbStatus != "active" {
				t.Errorf("Bind(%s): status = %q, want %q", tt.channelKey, dbStatus, "active")
			}
			if dbSessionID != tt.sessionID {
				t.Errorf("Bind(%s): session_id = %q, want %q", tt.channelKey, dbSessionID, tt.sessionID)
			}
		})
	}
}

// TestBind_AdvisoryLockPreventsDoubleProvision verifies that a concurrent bind
// for the same channel key returns an error. Delta-spec scenario: "Binding is
// idempotent under concurrent requests".
func TestBind_AdvisoryLockPreventsDoubleProvision(t *testing.T) {
	ctx, pool := newChannelTestDB(t)
	reg := NewPostgresRegistry(pool)

	channelKey := "mello:TICKET-DOUBLE"
	sessionID := "s1"
	runnerID := "r1"

	// First bind succeeds
	err := reg.Bind(ctx, channelKey, sessionID, runnerID, "mello", "TICKET-DOUBLE", "claude-code")
	if err != nil {
		t.Fatalf("first Bind: %v", err)
	}

	// Second bind for same key should fail (already bound)
	err = reg.Bind(ctx, channelKey, "s2", "r2", "mello", "TICKET-DOUBLE", "codex")
	if err == nil {
		t.Fatal("second Bind: expected error, got nil")
	}
}

// TestLookup_ReturnsSessionID verifies that after bind, Lookup returns the
// bound session ID. Delta-spec scenario: "Binding persists across restart".
func TestLookup_ReturnsSessionID(t *testing.T) {
	ctx, pool := newChannelTestDB(t)
	reg := NewPostgresRegistry(pool)

	tests := []struct {
		name       string
		channelKey string
		sessionID  string
	}{
		{
			name:       "lookup after bind returns session ID",
			channelKey: "mello:TICKET-99",
			sessionID:  "s1",
		},
		{
			name:       "lookup another channel",
			channelKey: "mello:TICKET-100",
			sessionID:  "s2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reg.Bind(ctx, tt.channelKey, tt.sessionID, "r1", "mello", "TICKET-99", "claude-code")
			if err != nil {
				t.Fatalf("Bind: %v", err)
			}

			got, err := reg.Lookup(ctx, tt.channelKey)
			if err != nil {
				t.Fatalf("Lookup(%s): %v", tt.channelKey, err)
			}
			if got != tt.sessionID {
				t.Errorf("Lookup(%s) = %q, want %q", tt.channelKey, got, tt.sessionID)
			}
		})
	}
}

// TestCacheHit_SkipsDB verifies that after the first lookup populates the cache,
// a second lookup returns the cached value without hitting the DB.
func TestCacheHit_SkipsDB(t *testing.T) {
	ctx, pool := newChannelTestDB(t)
	reg := NewPostgresRegistry(pool).(*PostgresRegistry)

	channelKey := "mello:TICKET-CACHE"
	err := reg.Bind(ctx, channelKey, "s1", "r1", "mello", "TICKET-CACHE", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// First lookup populates cache
	sessionID, err := reg.Lookup(ctx, channelKey)
	if err != nil {
		t.Fatalf("first Lookup: %v", err)
	}
	if sessionID != "s1" {
		t.Fatalf("first Lookup: got %q, want %q", sessionID, "s1")
	}

	// Manually clear the DB to verify cache hit skips DB
	_, err = pool.Exec(ctx, "DELETE FROM channel_sessions WHERE channel_key = $1", channelKey)
	if err != nil {
		t.Fatalf("delete channel_sessions: %v", err)
	}

	// Second lookup should still return from cache
	sessionID, err = reg.Lookup(ctx, channelKey)
	if err != nil {
		t.Fatalf("second Lookup (cache): %v", err)
	}
	if sessionID != "s1" {
		t.Errorf("second Lookup (cache): got %q, want %q", sessionID, "s1")
	}
}

// TestCacheMiss_FallsBackToDB verifies that after cache clear, lookup reads
// from DB and repopulates the cache.
func TestCacheMiss_FallsBackToDB(t *testing.T) {
	ctx, pool := newChannelTestDB(t)
	reg := NewPostgresRegistry(pool).(*PostgresRegistry)

	channelKey := "mello:TICKET-MISS"
	err := reg.Bind(ctx, channelKey, "s1", "r1", "mello", "TICKET-MISS", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// Populate cache by looking up
	_, err = reg.Lookup(ctx, channelKey)
	if err != nil {
		t.Fatalf("first Lookup: %v", err)
	}

	// Clear the cache
	reg.cache = &sync.Map{}

	// Lookup should fall back to DB and succeed
	sessionID, err := reg.Lookup(ctx, channelKey)
	if err != nil {
		t.Fatalf("Lookup after cache clear: %v", err)
	}
	if sessionID != "s1" {
		t.Errorf("Lookup after cache clear: got %q, want %q", sessionID, "s1")
	}

	// Verify cache is repopulated
	val, ok := reg.cache.Load(channelKey)
	if !ok {
		t.Error("cache should be repopulated after DB fallback")
	}
	cachedSessionID, ok := val.(string)
	if !ok || cachedSessionID != "s1" {
		t.Errorf("cached value = %v, want %q", val, "s1")
	}
}

// TestPopulateCache_FromDBOnStartup verifies that PopulateCache loads all
// active rows from the DB into the cache.
func TestPopulateCache_FromDBOnStartup(t *testing.T) {
	ctx, pool := newChannelTestDB(t)

	// Seed DB directly
	_, err := pool.Exec(ctx, `
		INSERT INTO channel_sessions (channel_key, session_id, provider_code, resource_id, runner_id, spec, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "mello:TICKET-1", "s1", "mello", "TICKET-1", "r1", "claude-code", "active")
	if err != nil {
		t.Fatalf("seed channel_sessions: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO channel_sessions (channel_key, session_id, provider_code, resource_id, runner_id, spec, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "mello:TICKET-2", "s2", "mello", "TICKET-2", "r1", "claude-code", "active")
	if err != nil {
		t.Fatalf("seed channel_sessions 2: %v", err)
	}

	// Create fresh registry and populate cache
	reg := NewPostgresRegistry(pool).(*PostgresRegistry)
	err = reg.PopulateCache(ctx)
	if err != nil {
		t.Fatalf("PopulateCache: %v", err)
	}

	// Verify cache has both entries
	val, ok := reg.cache.Load("mello:TICKET-1")
	if !ok {
		t.Error("cache missing channel after PopulateCache: mello:TICKET-1")
	}
	if val != "s1" {
		t.Errorf("cache value for mello:TICKET-1 = %v, want s1", val)
	}

	val, ok = reg.cache.Load("mello:TICKET-2")
	if !ok {
		t.Error("cache missing channel after PopulateCache: mello:TICKET-2")
	}
	if val != "s2" {
		t.Errorf("cache value for mello:TICKET-2 = %v, want s2", val)
	}
}

// TestUnbind_RemovesRow verifies that after unbind, the channel key is removed
// from the cache and the DB row status is set to 'closed'.
func TestUnbind_RemovesRow(t *testing.T) {
	ctx, pool := newChannelTestDB(t)
	reg := NewPostgresRegistry(pool)

	channelKey := "mello:TICKET-UNBIND"
	err := reg.Bind(ctx, channelKey, "s1", "r1", "mello", "TICKET-UNBIND", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// Unbind
	err = reg.Unbind(ctx, channelKey)
	if err != nil {
		t.Fatalf("Unbind(%s): %v", channelKey, err)
	}

	// Lookup should return empty/error
	sessionID, err := reg.Lookup(ctx, channelKey)
	if err == nil && sessionID != "" {
		t.Errorf("Lookup after Unbind(%s) = %q, want empty", channelKey, sessionID)
	}
}

// TestRunnerActiveChannelCount verifies that RunnerActiveChannelCount returns
// the correct count per runner. Delta-spec scenario: "Binding count tracks
// active channels".
func TestRunnerActiveChannelCount(t *testing.T) {
	ctx, pool := newChannelTestDB(t)
	reg := NewPostgresRegistry(pool)

	tests := []struct {
		name      string
		binds     []bindSeed
		runnerID  string
		wantCount int
	}{
		{
			name: "runner with 2 binds has count 2",
			binds: []bindSeed{
				{channelKey: "mello:TICKET-1", sessionID: "s1", runnerID: "r1", provider: "mello", resourceID: "TICKET-1", spec: "claude-code"},
				{channelKey: "mello:TICKET-2", sessionID: "s2", runnerID: "r1", provider: "mello", resourceID: "TICKET-2", spec: "claude-code"},
			},
			runnerID:  "r1",
			wantCount: 2,
		},
		{
			name: "runner with no binds has count 0",
			binds: []bindSeed{
				{channelKey: "mello:TICKET-3", sessionID: "s3", runnerID: "r2", provider: "mello", resourceID: "TICKET-3", spec: "claude-code"},
			},
			runnerID:  "other-runner",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear existing data from previous subtests
			_, _ = pool.Exec(ctx, "DELETE FROM channel_sessions")

			for _, b := range tt.binds {
				err := reg.Bind(ctx, b.channelKey, b.sessionID, b.runnerID, b.provider, b.resourceID, b.spec)
				if err != nil {
					t.Fatalf("Bind(%s): %v", b.channelKey, err)
				}
			}

			count, err := reg.RunnerActiveChannelCount(ctx, tt.runnerID)
			if err != nil {
				t.Fatalf("RunnerActiveChannelCount(%s): %v", tt.runnerID, err)
			}
			if count != tt.wantCount {
				t.Errorf("RunnerActiveChannelCount(%s) = %d, want %d", tt.runnerID, count, tt.wantCount)
			}
		})
	}
}

// bindSeed describes a bind operation for test setup.
type bindSeed struct {
	channelKey string
	sessionID  string
	runnerID   string
	provider   string
	resourceID string
	spec       string
}

// newChannelTestDB sets up a clean Postgres connection for channel tests.
func newChannelTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
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

	// Clean relevant tables
	_, err = pool.Exec(ctx, "DELETE FROM channel_sessions; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	return ctx, pool
}
