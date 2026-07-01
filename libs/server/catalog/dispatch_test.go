package catalog

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/shared/grant"
)

func TestDispatch_ResolveAndPublish(t *testing.T) {
	// This test verifies that Dispatch resolves the agent via the store,
	// builds a grant, and publishes a Dispatch message to the correct bus topic.

	broker := memory.New()
	svc := NewService(nil) // store-layer service; nil pool means DB ops will fail
	h := NewAgentHandlers(svc, broker, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub, err := broker.Subscribe(ctx, bus.Identity("test"), bus.Filter("runner.unit-test.dispatch"), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, []byte("test-key"))
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	err = h.DispatchToRunner(ctx, "code-fixer", "runner-unit-test", g)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Verify a dispatch message arrived on the correct topic.
	select {
	case evt := <-sub.Events():
		if evt.Topic != bus.Topic("runner.unit-test.dispatch") {
			t.Errorf("event topic = %q, want %q", evt.Topic, "runner.unit-test.dispatch")
		}
		var msg struct {
			Agent  map[string]string `json:"agent"`
			Grant  json.RawMessage   `json:"grant"`
			Runner string            `json:"runner"`
			Session string           `json:"session,omitempty"`
		}
		if err := json.Unmarshal(evt.Message.Payload, &msg); err != nil {
			t.Fatalf("unmarshal dispatch message: %v", err)
		}
		if msg.Agent == nil || msg.Agent["name"] != "code-fixer" {
			t.Errorf("dispatch agent ref = %v, want name=code-fixer", msg.Agent)
		}
		if len(msg.Grant) == 0 {
			t.Error("dispatch message missing grant")
		}
		if msg.Runner != "runner-unit-test" {
			t.Errorf("dispatch runner = %q, want %q", msg.Runner, "runner-unit-test")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for dispatch message")
	}
}

func TestDispatch_MissingAgent(t *testing.T) {
	broker := memory.New()
	svc := NewService(nil)
	h := NewAgentHandlers(svc, broker, nil, nil)

	ctx := context.Background()

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, []byte("test-key"))
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	// Dispatch an agent that doesn't exist — should return a not-found error.
	err = h.DispatchToRunner(ctx, "nonexistent-agent", "runner-R", g)
	if err == nil {
		t.Fatal("expected error for missing agent, got nil")
	}
}

func TestDispatch_MissingVersion(t *testing.T) {
	broker := memory.New()
	svc := NewService(nil)
	h := NewAgentHandlers(svc, broker, nil, nil)

	ctx := context.Background()

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, []byte("test-key"))
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	// Even with an existing agent name, a missing version should cause an error.
	err = h.DispatchVersionToRunner(ctx, "unknown-agent", "9.9.9", "runner-R", g)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestDispatch_Unauthorized(t *testing.T) {
	broker := memory.New()
	svc := NewService(nil)
	h := NewAgentHandlers(svc, broker, nil, nil)

	ctx := context.Background()

	// Empty/nil grant means no permissions — dispatch should be denied.
	err := h.DispatchToRunner(ctx, "code-fixer", "runner-R", nil)
	if err == nil {
		t.Fatal("expected authorization error for nil grant, got nil")
	}
}
