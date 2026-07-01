//go:build e2e

package e2e

// Feature: prebuilt agent-sandbox — interactive long-lived sessions on the
// `local` engine with live token streaming over the in-memory bus.
//
// This is the change c0026 acceptance suite (tasks 8.2 + 8.3). Unlike the BDD
// World scenarios in this package, these tests drive the REAL runner.Session
// API end-to-end against the REAL local sandbox engine and the in-memory bus —
// the same code path units 01–04 deliver. They mirror the multi-turn,
// re-exec-with-carried-history pattern from
// examples/remote-claude/scripts/e2e.py, but with a deterministic stub
// backend so the suite needs no installed Claude binary.
//
// Delta-spec scenarios covered (prebuilt-agent-sandbox/spec.md):
//   - "Multi-turn over one sandbox"      (Start once, ordered turns, stdin-not-argv)
//   - "Cancel an in-flight turn"         (interrupt, sandbox survives, next turn works)
//   - "Idle session is reaped"           (close + destroy past idle timeout)
//   - "Live token stream"                (token/message + one terminal per turn)
//   - "Late subscriber gets tail-then-live"

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runnerpkg "mework/libs/client/runner"
	"mework/libs/sandbox"
	"mework/libs/sandbox/engine/local"
	sbruntime "mework/libs/sandbox/runtime"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	sharedgrant "mework/libs/shared/grant"
	"mework/libs/shared/ports"
)

const (
	prebuiltOwner  = core.AccountID("e2e-owner")
	prebuiltTenant = core.TenantID("e2e-tenant")
	prebuiltRef    = "local-claude@1.0.0"
)

// prebuiltResolver is an in-tree DefinitionResolver that resolves the
// `local-claude` prebuilt definition to a local-engine bundle bound to a stub
// backend. It exercises the real reference-resolution seam without a catalog DB.
type prebuiltResolver struct {
	backendName string
}

func (r prebuiltResolver) ResolveDefinition(_ context.Context, ref string) (*sandbox.SandboxBundleMetadata, error) {
	if ref != prebuiltRef && ref != "local-claude@latest" {
		return nil, runnerpkg.ErrDefinitionNotFound
	}
	return &sandbox.SandboxBundleMetadata{
		Name:    "local-claude",
		Version: "1.0.0",
		Engine:  "local",
		Backend: r.backendName,
	}, nil
}

// countingLocalDriver wraps the REAL local engine driver so the e2e can assert
// the long-lived sandbox is started exactly once per session and destroyed on
// close/reap, while every turn still runs as a genuine host subprocess fed over
// stdin.
type countingLocalDriver struct {
	inner    ports.SandboxDriver
	starts   atomic.Int64
	destroys atomic.Int64
}

func (d *countingLocalDriver) Caps() core.SandboxCaps { return d.inner.Caps() }

func (d *countingLocalDriver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	d.starts.Add(1)
	return d.inner.Start(ctx, spec)
}

func (d *countingLocalDriver) Stop(ctx context.Context, id string) error {
	return d.inner.Stop(ctx, id)
}

func (d *countingLocalDriver) Destroy(ctx context.Context, id string) error {
	d.destroys.Add(1)
	return d.inner.Destroy(ctx, id)
}

// writeStubBackend writes an executable shell script that reads the whole turn
// from stdin (proving stdin-not-argv) and echoes a deterministic response, then
// returns the backend name and the dir to prepend to PATH. When block is true
// the stub blocks until its stdin closes / it is killed, modelling an in-flight
// turn that must be cancelled.
func writeStubBackend(t *testing.T, name string, block bool) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub backend uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	var script string
	if block {
		// Read one line then sleep forever; killed by context cancel.
		script = "#!/bin/sh\nhead -n1\nwhile true; do sleep 1; done\n"
	} else {
		script = "#!/bin/sh\nin=$(cat)\nprintf 'reply to: %s\\n' \"$in\"\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub backend: %v", err)
	}
	return name, dir
}

// newPrebuiltDeps wires the runner.SessionDeps over the REAL local engine
// (wrapped for counting), an in-memory bus, and a session manager with the given
// idle timeout. It returns the deps, the counting driver, the bus, and the
// session manager.
func newPrebuiltDeps(t *testing.T, backendName string, idle time.Duration) (runnerpkg.SessionDeps, *countingLocalDriver, bus.Broker, *session.Manager) {
	t.Helper()

	drv := &countingLocalDriver{inner: local.New()}
	broker := memory.New()
	cfg := session.DefaultConfig()
	if idle > 0 {
		cfg.IdleTimeout = idle
		cfg.ReapInterval = idle / 2
		if cfg.ReapInterval <= 0 {
			cfg.ReapInterval = idle
		}
	}
	mgr := session.NewManager(broker, cfg)
	t.Cleanup(mgr.Stop)

	deps := runnerpkg.SessionDeps{
		Resolver:   prebuiltResolver{backendName: backendName},
		ManagerFor: func(string) *sbruntime.Manager { return sbruntime.NewManager(drv) },
		Broker:     broker,
		Sessions:   mgr,
		GrantKey:   nil,
	}
	return deps, drv, broker, mgr
}

func prebuiltCaller(t *testing.T) runnerpkg.Caller {
	t.Helper()
	g, err := sharedgrant.NewGrant([]sharedgrant.Operation{sharedgrant.OpSpawn}, nil)
	if err != nil {
		t.Fatalf("new grant: %v", err)
	}
	return runnerpkg.Caller{Account: prebuiltOwner, Tenant: prebuiltTenant, Grant: g}
}

// sessionTopic is the bus topic the session publishes its turn events to.
func sessionTopic(id core.SessionID) bus.Topic {
	return bus.FormatTopic(bus.TopicSessionControl, string(id))
}

// onlySessionID returns the id of the single session visible to the test
// tenant. The runner.Session API publishes to a per-session topic, so the e2e
// derives that id from the tenant-scoped list rather than reaching into the
// session value.
func onlySessionID(t *testing.T, sess *runnerpkg.Session, caller runnerpkg.Caller) core.SessionID {
	t.Helper()
	list, err := sess.List(context.Background(), caller)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 session for tenant %q, got %d", caller.Tenant, len(list))
	}
	return list[0].ID
}

// drainTurn drains a subscription until one terminal (done/error) event arrives
// or the deadline elapses, returning the events in delivery order.
func drainTurn(t *testing.T, sub bus.Subscription, deadline time.Duration) []session.ChatEvent {
	t.Helper()
	var out []session.ChatEvent
	timer := time.After(deadline)
	for {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				return out
			}
			ce, err := runnerpkg.DecodeChatEvent(ev.Message.Payload)
			if err != nil {
				t.Fatalf("decode chat event %q: %v", string(ev.Message.Payload), err)
			}
			out = append(out, ce)
			if ce.Kind == session.EventDone || ce.Kind == session.EventError {
				return out
			}
		case <-timer:
			return out
		}
	}
}

func assertOneTerminalLast(t *testing.T, events []session.ChatEvent, want session.ChatEventKind) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("no events published for the turn")
	}
	terminals := 0
	for i, ev := range events {
		if ev.Kind == session.EventDone || ev.Kind == session.EventError {
			terminals++
			if i != len(events)-1 {
				t.Errorf("terminal event at index %d is not last (of %d)", i, len(events))
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("expected exactly one terminal event, got %d: %+v", terminals, events)
	}
	if last := events[len(events)-1]; last.Kind != want {
		t.Errorf("terminal kind = %q, want %q", last.Kind, want)
	}
}

// TestPrebuiltAgentSandbox_InteractiveSessionE2E is the cross-package acceptance
// for an interactive, long-lived session on the local engine: multi-turn over
// one sandbox, cancel of an in-flight turn, and idle reap — each with live event
// streaming over the in-memory bus.
func TestPrebuiltAgentSandbox_InteractiveSessionE2E(t *testing.T) {
	t.Run("multi-turn over one long-lived sandbox", func(t *testing.T) {
		backend, stubDir := writeStubBackend(t, "claude", false)
		t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		deps, drv, broker, _ := newPrebuiltDeps(t, backend, 0)
		caller := prebuiltCaller(t)
		ctx := context.Background()

		sess, err := runnerpkg.OpenSession(ctx, prebuiltRef, caller, deps)
		if err != nil {
			t.Fatalf("OpenSession: %v", err)
		}
		t.Cleanup(func() { _ = sess.Close(ctx, caller) })

		st, err := sess.Status(ctx, caller)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if st != core.SessionActive {
			t.Fatalf("status after open = %q, want %q", st, core.SessionActive)
		}

		sub, err := broker.Subscribe(ctx, "watcher", bus.Filter(sessionTopic(onlySessionID(t, sess, caller))), "")
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		t.Cleanup(func() { _ = sub.Close() })

		// Two turns over the SAME long-lived sandbox. Each turn re-execs the
		// backend with the carried turn content over stdin (the remote-claude
		// multi-turn pattern), so the second runs after the first.
		turns := []string{"first: write a palindrome checker", "second: now handle unicode"}
		for _, turn := range turns {
			if err := sess.Send(ctx, caller, turn); err != nil {
				t.Fatalf("send %q: %v", turn, err)
			}
			events := drainTurn(t, sub, 5*time.Second)
			assertOneTerminalLast(t, events, session.EventDone)

			joined := ""
			for _, ev := range events {
				joined += ev.Content
			}
			// stdin-not-argv: the stub echoes whatever it read on stdin, so the
			// turn content must surface in the streamed events.
			if !strings.Contains(joined, turn) {
				t.Errorf("turn content not streamed back; got %q want substring %q", joined, turn)
			}
		}

		if got := drv.starts.Load(); got != 1 {
			t.Errorf("Start called %d times, want exactly 1 (one long-lived sandbox per session)", got)
		}
	})

	t.Run("cancel an in-flight turn keeps the session usable", func(t *testing.T) {
		backend, stubDir := writeStubBackend(t, "claude", true)
		t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		deps, drv, _, _ := newPrebuiltDeps(t, backend, 0)
		caller := prebuiltCaller(t)
		ctx := context.Background()

		sess, err := runnerpkg.OpenSession(ctx, prebuiltRef, caller, deps)
		if err != nil {
			t.Fatalf("OpenSession: %v", err)
		}
		t.Cleanup(func() { _ = sess.Close(ctx, caller) })

		// Launch a turn whose backend blocks until cancelled.
		sendErr := make(chan error, 1)
		go func() { sendErr <- sess.Send(ctx, caller, "long-running turn") }()

		// Give the blocking subprocess a moment to actually start.
		time.Sleep(150 * time.Millisecond)

		if err := sess.Cancel(ctx, caller); err != nil {
			t.Fatalf("cancel: %v", err)
		}

		select {
		case <-sendErr:
		case <-time.After(3 * time.Second):
			t.Fatal("in-flight turn did not stop after cancel")
		}

		// Sandbox must survive cancel — not destroyed, not restarted.
		if got := drv.destroys.Load(); got != 0 {
			t.Errorf("cancel destroyed the sandbox %d time(s); it must stay alive", got)
		}
		if got := drv.starts.Load(); got != 1 {
			t.Errorf("cancel must not restart the sandbox: Start called %d times", got)
		}

		// The session must still accept a follow-up turn on the SAME sandbox.
		// Swap the stub for a non-blocking one so the follow-up completes.
		_, fastDir := writeStubBackend(t, backend, false)
		t.Setenv("PATH", fastDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if err := sess.Send(ctx, caller, "follow-up turn"); err != nil {
			t.Fatalf("follow-up send after cancel: %v", err)
		}
		if got := drv.starts.Load(); got != 1 {
			t.Errorf("follow-up turn must reuse the sandbox: Start called %d times", got)
		}
	})

	t.Run("idle session is reaped and its sandbox destroyed", func(t *testing.T) {
		backend, stubDir := writeStubBackend(t, "claude", false)
		t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		deps, drv, _, _ := newPrebuiltDeps(t, backend, 60*time.Millisecond)
		caller := prebuiltCaller(t)
		ctx := context.Background()

		sess, err := runnerpkg.OpenSession(ctx, prebuiltRef, caller, deps)
		if err != nil {
			t.Fatalf("OpenSession: %v", err)
		}
		if err := sess.Send(ctx, caller, "a turn then go idle"); err != nil {
			t.Fatalf("send: %v", err)
		}

		// Wait past the idle timeout for the reaper to close the session; the
		// sandbox must then be destroyed.
		deadline := time.After(3 * time.Second)
		for {
			st, _ := sess.Status(ctx, caller)
			if st == core.SessionClosed && drv.destroys.Load() >= 1 {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("idle session not reaped+destroyed: status=%q destroys=%d", st, drv.destroys.Load())
			case <-time.After(15 * time.Millisecond):
			}
		}
	})
}

// TestPrebuiltAgentSandbox_EventStreamOrderingAndTailThenLive asserts the live
// observability contract: ordered token→message→terminal per turn, exactly one
// terminal, and a late subscriber receiving the buffered tail then the live
// stream (tasks 8.3).
func TestPrebuiltAgentSandbox_EventStreamOrderingAndTailThenLive(t *testing.T) {
	backend, stubDir := writeStubBackend(t, "claude", false)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	deps, _, broker, _ := newPrebuiltDeps(t, backend, 0)
	caller := prebuiltCaller(t)
	ctx := context.Background()

	sess, err := runnerpkg.OpenSession(ctx, prebuiltRef, caller, deps)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close(ctx, caller) })

	topic := sessionTopic(onlySessionID(t, sess, caller))

	// First turn happens BEFORE the late subscriber attaches.
	if err := sess.Send(ctx, caller, "first turn before subscribe"); err != nil {
		t.Fatalf("send first turn: %v", err)
	}

	// Late subscriber attaches from the beginning (last_event_id="") so it must
	// receive the buffered tail of the first turn, in order, ending in a terminal.
	late, err := broker.Subscribe(ctx, "late", bus.Filter(topic), "")
	if err != nil {
		t.Fatalf("subscribe late: %v", err)
	}
	t.Cleanup(func() { _ = late.Close() })

	tail := drainTurn(t, late, 5*time.Second)
	assertOneTerminalLast(t, tail, session.EventDone)
	assertTokenBeforeMessage(t, tail)

	// Then it continues with the LIVE stream of a second turn.
	if err := sess.Send(ctx, caller, "second turn after subscribe"); err != nil {
		t.Fatalf("send second turn: %v", err)
	}
	live := drainTurn(t, late, 5*time.Second)
	assertOneTerminalLast(t, live, session.EventDone)
	assertTokenBeforeMessage(t, live)

	joined := ""
	for _, ev := range live {
		joined += ev.Content
	}
	if !strings.Contains(joined, "second turn after subscribe") {
		t.Errorf("late subscriber missed live second-turn content, got %q", joined)
	}
}

// assertTokenBeforeMessage verifies the per-turn ordering invariant: any token
// events precede the message event, which precedes the terminal.
func assertTokenBeforeMessage(t *testing.T, events []session.ChatEvent) {
	t.Helper()
	seenMessage := false
	for _, ev := range events {
		if ev.Kind == session.EventToken && seenMessage {
			t.Errorf("token event arrived after a message event: %+v", events)
		}
		if ev.Kind == session.EventMessage {
			seenMessage = true
		}
	}
}

// TestPrebuiltAgentSandbox_UnknownReferenceRejected confirms a reference that
// resolves to no definition is rejected before any sandbox is started — the
// one-shot RunByReference and the interactive OpenSession path both fail fast.
func TestPrebuiltAgentSandbox_UnknownReferenceRejected(t *testing.T) {
	backend, stubDir := writeStubBackend(t, "claude", false)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	deps, drv, _, _ := newPrebuiltDeps(t, backend, 0)
	caller := prebuiltCaller(t)
	ctx := context.Background()

	if _, err := runnerpkg.OpenSession(ctx, "does-not-exist@9.9.9", caller, deps); err == nil {
		t.Fatal("OpenSession with unknown reference should be rejected")
	}
	if got := drv.starts.Load(); got != 0 {
		t.Errorf("no sandbox should start for an unknown reference, Start called %d times", got)
	}

	res := runnerpkg.RunByReference(ctx, "does-not-exist@9.9.9", "hello", runnerpkg.RunDeps{
		Resolver:   deps.Resolver,
		ManagerFor: func(string) *sbruntime.Manager { return sbruntime.NewManager(drv) },
	})
	if res.Error == "" {
		t.Errorf("RunByReference with unknown reference should return a not-found error, got %+v", res)
	}
}
