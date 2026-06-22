package catalog

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mework/libs/client/runner"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/shared/grant"
)

// TestDispatchSessionToRunner publishes an open-session dispatch carrying the
// session id, owner, tenant, and a pull+spawn grant to the runner's dispatch
// topic, and asserts the topic matches the daemon Engine's subscription topic
// for the same runner id.
func TestDispatchSessionToRunner(t *testing.T) {
	broker := memory.New()
	svc := NewService(nil)
	h := NewAgentHandlers(svc, broker, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const runnerID = "rnr-1"
	const sessionID = "sess-xyz"
	const owner = "acct-1"
	const tenant = "tenant-1"

	// The daemon Engine subscribes to this topic for the same runner id.
	wantTopic := runner.DispatchTopic(runnerID)

	sub, err := broker.Subscribe(ctx, bus.Identity("test"), bus.Filter(string(wantTopic)), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, nil)
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	if err := h.DispatchSessionToRunner(ctx, "code-fixer", runnerID, sessionID, owner, tenant, "/ws/proj", g); err != nil {
		t.Fatalf("DispatchSessionToRunner: %v", err)
	}

	select {
	case evt := <-sub.Events():
		if string(evt.Topic) != string(wantTopic) {
			t.Errorf("topic = %q, want %q", evt.Topic, wantTopic)
		}
		var msg struct {
			Agent   map[string]string `json:"agent"`
			Grant   json.RawMessage   `json:"grant"`
			Session string            `json:"session"`
			Owner   string            `json:"owner"`
			Tenant    string          `json:"tenant"`
			Runner    string          `json:"runner"`
			Workspace string          `json:"workspace"`
		}
		if err := json.Unmarshal(evt.Message.Payload, &msg); err != nil {
			t.Fatalf("unmarshal dispatch: %v", err)
		}
		if msg.Session != sessionID {
			t.Errorf("session = %q, want %q", msg.Session, sessionID)
		}
		if msg.Workspace != "/ws/proj" {
			t.Errorf("workspace = %q, want %q", msg.Workspace, "/ws/proj")
		}
		if msg.Owner != owner {
			t.Errorf("owner = %q, want %q", msg.Owner, owner)
		}
		if msg.Tenant != tenant {
			t.Errorf("tenant = %q, want %q", msg.Tenant, tenant)
		}
		if msg.Agent["name"] != "code-fixer" {
			t.Errorf("agent = %v, want name=code-fixer", msg.Agent)
		}
		if len(msg.Grant) == 0 {
			t.Error("missing grant")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for dispatch")
	}
}

// TestDispatchSessionToRunner_NilGrant rejects a dispatch without a grant.
func TestDispatchSessionToRunner_NilGrant(t *testing.T) {
	broker := memory.New()
	h := NewAgentHandlers(NewService(nil), broker, nil, nil)
	err := h.DispatchSessionToRunner(context.Background(), "code-fixer", "rnr-1", "sess", "o", "t", "", nil)
	if err == nil {
		t.Fatal("expected error for nil grant")
	}
}
