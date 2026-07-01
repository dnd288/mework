package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// collectEvents drains a subscription's channel until a terminal (done/error)
// event is seen or the deadline elapses, returning the parsed ChatEvents in
// delivery order.
func collectEvents(t *testing.T, sub bus.Subscription, deadline time.Duration) []session.ChatEvent {
	t.Helper()
	var out []session.ChatEvent
	timer := time.After(deadline)
	for {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				return out
			}
			ce, perr := DecodeChatEvent(ev.Message.Payload)
			if perr != nil {
				t.Fatalf("decode chat event %q: %v", string(ev.Message.Payload), perr)
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

func TestSessionEvents_PublishedPerTurn(t *testing.T) {
	tests := []struct {
		name         string
		backendOut   string // simulated backend stdout/stderr for the turn
		backendErr   bool   // the turn's backend exits with an error
		wantTerminal session.ChatEventKind
		wantContent  string // substring expected in a non-terminal event ("" = skip)
		wantRaw      bool   // unparseable output must fall back to a raw event
	}{
		{
			name:         "token then message then done",
			backendOut:   "hello world",
			wantTerminal: session.EventDone,
			wantContent:  "hello",
		},
		{
			name:         "errored turn emits a single error terminal",
			backendOut:   "boom",
			backendErr:   true,
			wantTerminal: session.EventError,
		},
		{
			name:         "unparseable output falls back to a raw event",
			backendOut:   "\x00\x01 not-json garbage \xff",
			wantTerminal: session.EventDone,
			wantRaw:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := memory.New()
			ctx := context.Background()
			sessionID := core.SessionID("sess-1")
			topic := bus.FormatTopic(bus.TopicSessionControl, string(sessionID))

			sub, err := broker.Subscribe(ctx, "subscriber", bus.Filter(topic), "")
			if err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			t.Cleanup(func() { _ = sub.Close() })

			pub := NewEventPublisher(broker, sessionID)
			if err := pub.PublishTurn(ctx, TurnResult{
				Output: tt.backendOut,
				Failed: tt.backendErr,
			}); err != nil {
				t.Fatalf("publish turn: %v", err)
			}

			events := collectEvents(t, sub, time.Second)
			if len(events) == 0 {
				t.Fatal("no events were published for the turn")
			}

			// Exactly one terminal, and it must be last.
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
				t.Fatalf("expected exactly one terminal event, got %d (%+v)", terminals, events)
			}
			last := events[len(events)-1]
			if last.Kind != tt.wantTerminal {
				t.Errorf("terminal kind = %q, want %q", last.Kind, tt.wantTerminal)
			}

			// ordering: any token(s) before message before terminal.
			seenMessage := false
			for _, ev := range events {
				if ev.Kind == session.EventToken && seenMessage {
					t.Errorf("token event arrived after a message event: %+v", events)
				}
				if ev.Kind == session.EventMessage {
					seenMessage = true
				}
			}

			if tt.wantContent != "" {
				joined := ""
				for _, ev := range events {
					joined += ev.Content
				}
				if !strings.Contains(joined, tt.wantContent) {
					t.Errorf("event content %q does not contain %q", joined, tt.wantContent)
				}
			}

			if tt.wantRaw {
				// The raw backend output must survive somewhere in the stream
				// rather than being silently dropped.
				joined := ""
				for _, ev := range events {
					joined += ev.Content
				}
				if joined == "" {
					t.Error("unparseable output was dropped; expected a raw fallback event")
				}
			}
		})
	}
}

func TestSessionEvents_LateSubscriberTailThenLive(t *testing.T) {
	broker := memory.New()
	ctx := context.Background()
	sessionID := core.SessionID("sess-late")
	topic := bus.FormatTopic(bus.TopicSessionControl, string(sessionID))

	pub := NewEventPublisher(broker, sessionID)

	// First turn happens entirely before the late subscriber attaches.
	if err := pub.PublishTurn(ctx, TurnResult{Output: "first turn"}); err != nil {
		t.Fatalf("publish first turn: %v", err)
	}

	// Late subscriber attaches from the beginning (last_event_id="") and so must
	// receive the buffered tail of the first turn...
	sub, err := broker.Subscribe(ctx, "late", bus.Filter(topic), "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	tail := collectEvents(t, sub, time.Second)
	if len(tail) == 0 {
		t.Fatal("late subscriber received no buffered tail events")
	}
	if tail[len(tail)-1].Kind != session.EventDone {
		t.Errorf("buffered tail did not end with a terminal done event: %+v", tail)
	}

	// ...then continue with the live stream of a second turn.
	if err := pub.PublishTurn(ctx, TurnResult{Output: "second turn"}); err != nil {
		t.Fatalf("publish second turn: %v", err)
	}
	live := collectEvents(t, sub, time.Second)
	if len(live) == 0 {
		t.Fatal("late subscriber received no live events for the second turn")
	}
	joined := ""
	for _, ev := range live {
		joined += ev.Content
	}
	if !strings.Contains(joined, "second") {
		t.Errorf("live stream missing second-turn content, got %q", joined)
	}
}

func TestSessionEvents_StatusAndList(t *testing.T) {
	deps, _, _ := newSessionDeps(t, 0, false)
	ctx := context.Background()
	caller := ownerCaller(t)

	sess, err := OpenSession(ctx, "local-claude@1.0.0", caller, deps)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close(ctx, caller) })

	// Status reflects the active session.
	st, err := sess.Status(ctx, caller)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st != core.SessionActive {
		t.Errorf("status = %q, want %q", st, core.SessionActive)
	}

	// List is authorized + tenant-scoped: the owner sees its own tenant's session.
	got, err := sess.List(ctx, caller)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List for tenant %q returned %d sessions, want 1", caller.Tenant, len(got))
	}

	// A caller authenticated in a different tenant sees none of it — the tenant
	// is derived from the caller, not a supplied argument, so it cannot be spoofed.
	g, err := grant.NewGrant([]grant.Operation{grant.OpSpawn}, nil)
	if err != nil {
		t.Fatalf("new grant: %v", err)
	}
	otherCaller := Caller{Account: core.AccountID("acct-b"), Tenant: core.TenantID("tenant-b"), Grant: g}
	other, err := sess.List(ctx, otherCaller)
	if err != nil {
		t.Fatalf("list other tenant: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("List for tenant-b returned %d sessions, want 0 (tenant isolation)", len(other))
	}

	// A caller without a grant is denied (list is an authorized operation).
	if _, err := sess.List(ctx, Caller{Account: testOwner, Tenant: testTenant}); err == nil {
		t.Error("List without a grant should be denied")
	}
}
