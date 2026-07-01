package session

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRole_Values(t *testing.T) {
	tests := []struct {
		role Role
		want string
	}{
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{RoleSystem, "system"},
	}
	for _, tc := range tests {
		if string(tc.role) != tc.want {
			t.Errorf("Role(%q) = %q, want %q", tc.want, string(tc.role), tc.want)
		}
	}
}

func TestChatMessage_Roundtrip(t *testing.T) {
	msg := ChatMessage{Role: RoleUser, Content: "hello"}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ChatMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Role != msg.Role || got.Content != msg.Content {
		t.Errorf("roundtrip = %+v, want %+v", got, msg)
	}
}

func TestChatEventKind_Values(t *testing.T) {
	tests := []struct {
		kind ChatEventKind
		want string
	}{
		{EventToken, "token"},
		{EventMessage, "message"},
		{EventDone, "done"},
		{EventError, "error"},
	}
	for _, tc := range tests {
		if string(tc.kind) != tc.want {
			t.Errorf("ChatEventKind(%q) = %q, want %q", tc.want, string(tc.kind), tc.want)
		}
	}
}

func TestChatEvent_Roundtrip(t *testing.T) {
	ev := ChatEvent{Kind: EventToken, Content: "Hello"}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ChatEvent
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Kind != ev.Kind || got.Content != ev.Content {
		t.Errorf("roundtrip = %+v, want %+v", got, ev)
	}
}

func TestChatEvent_DoneHasNoContent(t *testing.T) {
	// A done event should have empty content.
	ev := ChatEvent{Kind: EventDone}
	if ev.Content != "" {
		t.Errorf("done event should have empty content, got %q", ev.Content)
	}
}

func TestConversation_Interface(t *testing.T) {
	// Compile-time check: ensure the interface has the expected methods.
	var _ Conversation = &mockConversation{}
}

// mockConversation implements Conversation for compile-time interface check.
type mockConversation struct{}

func (m *mockConversation) Send(ctx context.Context, msg ChatMessage) error { return nil }
func (m *mockConversation) Stream() <-chan ChatEvent { return make(chan ChatEvent) }
func (m *mockConversation) History(ctx context.Context) ([]ChatMessage, error) { return nil, nil }
func (m *mockConversation) Cancel(ctx context.Context)                         {}
