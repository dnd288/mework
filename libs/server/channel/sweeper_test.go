package channel

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/store"
)

type memoryLogHandler struct {
	mu     sync.Mutex
	logs   []string
	level  slog.Level
}

func (h *memoryLogHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *memoryLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var b strings.Builder
	_ = r.Message
	b.WriteString(r.Message)
	h.logs = append(h.logs, b.String())
	return nil
}

func (h *memoryLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *memoryLogHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *memoryLogHandler) contains(sub string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, l := range h.logs {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestSweeper_ClosesOrphanedChannel(t *testing.T) {
	ctx, pool := newSweeperTestDB(t)
	reg := NewPostgresRegistry(pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000010", "00000000-0000-0000-0000-000000000020", "wrk1", "Worker 1", "online", "lookup-online")
	if err != nil {
		t.Fatalf("seed online runner: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000010", "00000000-0000-0000-0000-000000000020", "wrk2", "Worker 2", "offline", "lookup-offline")
	if err != nil {
		t.Fatalf("seed offline runner: %v", err)
	}

	err = reg.Bind(ctx, "mello:TICKET-ONLINE", "sess-online", "00000000-0000-0000-0000-000000000001", "mello", "TICKET-ONLINE", "claude-code")
	if err != nil {
		t.Fatalf("Bind online channel: %v", err)
	}

	err = reg.Bind(ctx, "mello:TICKET-OFFLINE", "sess-offline", "00000000-0000-0000-0000-000000000002", "mello", "TICKET-OFFLINE", "claude-code")
	if err != nil {
		t.Fatalf("Bind offline channel: %v", err)
	}

	sweeper := NewSweeper(pool, reg, 100*time.Millisecond)
	if err := sweeper.Run(ctx); err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}

	statusOnline, err := reg.Status(ctx, "mello:TICKET-ONLINE")
	if err != nil {
		t.Fatalf("Status online: %v", err)
	}
	if statusOnline != StatusActive {
		t.Errorf("online channel status = %q, want %q", statusOnline, StatusActive)
	}

	statusOffline, err := reg.Status(ctx, "mello:TICKET-OFFLINE")
	if err != nil {
		t.Fatalf("Status offline: %v", err)
	}
	if statusOffline != StatusClosed {
		t.Errorf("offline channel status = %q, want %q", statusOffline, StatusClosed)
	}

	var closedAt *time.Time
	err = pool.QueryRow(ctx,
		"SELECT closed_at FROM channel_sessions WHERE channel_key = $1",
		"mello:TICKET-OFFLINE",
	).Scan(&closedAt)
	if err != nil {
		t.Fatalf("query closed_at: %v", err)
	}
	if closedAt == nil {
		t.Error("expected closed_at to be set on orphaned channel, got nil")
	}
}

func TestSweeper_LeavesActiveChannels(t *testing.T) {
	ctx, pool := newSweeperTestDB(t)
	reg := NewPostgresRegistry(pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "00000000-0000-0000-0000-000000000003", "00000000-0000-0000-0000-000000000010", "00000000-0000-0000-0000-000000000020", "wrk1", "Worker 1", "online", "lookup-healthy")
	if err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	err = reg.Bind(ctx, "mello:TICKET-HEALTHY", "sess-1", "00000000-0000-0000-0000-000000000003", "mello", "TICKET-HEALTHY", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	sweeper := NewSweeper(pool, reg, 100*time.Millisecond)
	if err := sweeper.Run(ctx); err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}

	status, err := reg.Status(ctx, "mello:TICKET-HEALTHY")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != StatusActive {
		t.Errorf("healthy channel status = %q, want %q", status, StatusActive)
	}
}

func TestSweeper_LogsOrphanedClosures(t *testing.T) {
	ctx, pool := newSweeperTestDB(t)
	reg := NewPostgresRegistry(pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "00000000-0000-0000-0000-000000000004", "00000000-0000-0000-0000-000000000010", "00000000-0000-0000-0000-000000000020", "wrk1", "Worker 1", "offline", "lookup-gone")
	if err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	err = reg.Bind(ctx, "mello:TICKET-LOG", "sess-log", "00000000-0000-0000-0000-000000000004", "mello", "TICKET-LOG", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	logHandler := &memoryLogHandler{level: slog.LevelInfo}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(logHandler))
	defer slog.SetDefault(origLogger)

	sweeper := NewSweeper(pool, reg, 100*time.Millisecond)
	if err := sweeper.Run(ctx); err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}

	if !logHandler.contains("sweeper closed orphaned channel") {
		t.Error("sweeper did not log orphaned channel closure")
	}
	if !logHandler.contains("closed") {
		t.Error("sweeper did not log 'closed' action")
	}
}

func TestSweeper_MissingRunnerClosed(t *testing.T) {
	ctx, pool := newSweeperTestDB(t)
	reg := NewPostgresRegistry(pool)

	err := reg.Bind(ctx, "mello:TICKET-GHOST", "sess-ghost", "runner-nonexistent", "mello", "TICKET-GHOST", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	sweeper := NewSweeper(pool, reg, 100*time.Millisecond)
	if err := sweeper.Run(ctx); err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}

	status, err := reg.Status(ctx, "mello:TICKET-GHOST")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != StatusClosed {
		t.Errorf("ghost-runner channel status = %q, want %q", status, StatusClosed)
	}
}

func TestSweeper_RunTwiceIdempotent(t *testing.T) {
	ctx, pool := newSweeperTestDB(t)
	reg := NewPostgresRegistry(pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "00000000-0000-0000-0000-000000000005", "00000000-0000-0000-0000-000000000010", "00000000-0000-0000-0000-000000000020", "wrk1", "Worker 1", "offline", "lookup-idem")
	if err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	err = reg.Bind(ctx, "mello:TICKET-IDEM", "sess-idem", "00000000-0000-0000-0000-000000000005", "mello", "TICKET-IDEM", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	sweeper := NewSweeper(pool, reg, 100*time.Millisecond)

	if err := sweeper.Run(ctx); err != nil {
		t.Fatalf("first sweeper.Run: %v", err)
	}
	status, err := reg.Status(ctx, "mello:TICKET-IDEM")
	if err != nil {
		t.Fatalf("Status after first run: %v", err)
	}
	if status != StatusClosed {
		t.Fatalf("after first run: status = %q, want %q", status, StatusClosed)
	}

	if err := sweeper.Run(ctx); err != nil {
		t.Fatalf("second sweeper.Run: %v", err)
	}
	status, err = reg.Status(ctx, "mello:TICKET-IDEM")
	if err != nil {
		t.Fatalf("Status after second run: %v", err)
	}
	if status != StatusClosed {
		t.Errorf("after second run: status = %q, want %q", status, StatusClosed)
	}
}

func TestSweeper_StartStopsGracefully(t *testing.T) {
	ctx, pool := newSweeperTestDB(t)
	reg := NewPostgresRegistry(pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO runtimes (id, tenant_id, account_id, code, label, status, token_lookup)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "00000000-0000-0000-0000-000000000006", "00000000-0000-0000-0000-000000000010", "00000000-0000-0000-0000-000000000020", "wrk1", "Worker 1", "offline", "lookup-start")
	if err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	err = reg.Bind(ctx, "mello:TICKET-START", "sess-start", "00000000-0000-0000-0000-000000000006", "mello", "TICKET-START", "claude-code")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	sweeper := NewSweeper(pool, reg, 50*time.Millisecond)

	ctxStart, cancel := context.WithCancel(ctx)
	sweeper.Start(ctxStart)

	time.Sleep(150 * time.Millisecond)

	status, err := reg.Status(ctx, "mello:TICKET-START")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != StatusClosed {
		t.Errorf("after sweeper start: status = %q, want %q", status, StatusClosed)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

func newSweeperTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping sweeper integration test")
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

	_, err = pool.Exec(ctx,
		"DELETE FROM channel_sessions; DELETE FROM runtimes; DELETE FROM accounts; DELETE FROM tenants;",
	)
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	// Seed tenant and account so FK constraints are satisfied.
	_, err = pool.Exec(ctx, "INSERT INTO tenants (id, name) VALUES ('00000000-0000-0000-0000-000000000010', 'sweeper-test-tenant') ON CONFLICT (id) DO NOTHING")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO accounts (id, name) VALUES ('00000000-0000-0000-0000-000000000020', 'sweeper-test-account') ON CONFLICT (id) DO NOTHING")
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	return ctx, pool
}
