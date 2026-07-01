package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"mework/libs/client/subscribe"
	"mework/libs/shared/transport"
)

// TestEngine_RoutesSessionDispatch asserts that the dispatch worker routes a
// dispatch carrying a non-empty Session id to the session path (not the one-shot
// path), and that a second dispatch for the same session id is idempotent — it is
// acknowledged without re-opening the session. Realises delta-spec scenarios
// "Open-session dispatch starts one long-lived sandbox" and "Duplicate dispatch
// is idempotent".
func TestEngine_RoutesSessionDispatch(t *testing.T) {
	// Hub stub: accepts the ack for the idempotent duplicate dispatch.
	var ackCount int
	var ackMu sync.Mutex
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ackMu.Lock()
		ackCount++
		ackMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	eng := NewEngine("test-runner", "test-secret", hub.URL, "http://catalog.local")
	eng.client = subscribe.NewClient(hub.URL, 5*time.Second)

	var mu sync.Mutex
	var sessionOpens []string

	// Intercept the session path: record each id the engine asks to open and
	// mirror the registry behavior so the second dispatch sees it as open.
	origOpen := openSessionDispatch
	openSessionDispatch = func(_ context.Context, e *Engine, d transport.Dispatch, _ string, _ processOpts) error {
		mu.Lock()
		sessionOpens = append(sessionOpens, d.Session)
		mu.Unlock()
		e.registerSession(d.Session, &Session{})
		return nil
	}
	t.Cleanup(func() { openSessionDispatch = origOpen })

	ctx := context.Background()
	opts := processOpts{hubURL: eng.hubURL, catalogURL: eng.catalogURL, secret: eng.secret, client: eng.client}

	d := transport.Dispatch{Session: "sess-xyz", Runner: "test-runner", Owner: "owner-acct", Tenant: "tenant-a"}

	// First dispatch: routed to the session path, opens once.
	if err := eng.routeDispatch(ctx, d, "evt-1", opts); err != nil {
		t.Fatalf("first routeDispatch: %v", err)
	}
	// Second dispatch for the same session id: must be idempotent (no re-open).
	if err := eng.routeDispatch(ctx, d, "evt-2", opts); err != nil {
		t.Fatalf("second routeDispatch: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sessionOpens) != 1 {
		t.Fatalf("session opened %d time(s) (%v), want exactly 1 (idempotent on redelivery)", len(sessionOpens), sessionOpens)
	}
	if sessionOpens[0] != "sess-xyz" {
		t.Errorf("opened session id = %q, want %q", sessionOpens[0], "sess-xyz")
	}
	ackMu.Lock()
	defer ackMu.Unlock()
	if ackCount != 1 {
		t.Errorf("duplicate dispatch acked %d time(s), want exactly 1", ackCount)
	}
}

// TestEngine_NonSessionDispatchStaysOneShot asserts a dispatch without a session
// id is NOT routed to the session path; it stays on the one-shot path.
func TestEngine_NonSessionDispatchStaysOneShot(t *testing.T) {
	eng := NewEngine("test-runner", "test-secret", "http://hub.local", "http://catalog.local")

	var opened bool
	origOpen := openSessionDispatch
	openSessionDispatch = func(context.Context, *Engine, transport.Dispatch, string, processOpts) error {
		opened = true
		return nil
	}
	t.Cleanup(func() { openSessionDispatch = origOpen })

	// One-shot path: intercept processDispatch so no real HTTP is attempted.
	var oneShot bool
	origOneShot := processDispatchFn
	processDispatchFn = func(context.Context, transport.Dispatch, string, processOpts) error {
		oneShot = true
		return nil
	}
	t.Cleanup(func() { processDispatchFn = origOneShot })

	ctx := context.Background()
	opts := processOpts{hubURL: eng.hubURL, catalogURL: eng.catalogURL, secret: eng.secret, client: eng.client}

	d := transport.Dispatch{Runner: "test-runner"} // no Session
	if err := eng.routeDispatch(ctx, d, "evt-1", opts); err != nil {
		t.Fatalf("routeDispatch: %v", err)
	}
	if opened {
		t.Error("non-session dispatch was wrongly routed to the session path")
	}
	if !oneShot {
		t.Error("non-session dispatch was not routed to the one-shot path")
	}
}
