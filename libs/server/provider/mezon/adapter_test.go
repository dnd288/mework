package mezon

import (
	"context"
	"errors"
	"testing"

	"mework/libs/server/provider"
)

// ---------------------------------------------------------------------------
// Mock BotSender
// ---------------------------------------------------------------------------

// mockBotSender implements the BotSender interface for testing the adapter.
type mockBotSender struct {
	lastChannelID string
	lastBody      string
	sendErr       error
}

func (m *mockBotSender) SendMessage(ctx context.Context, channelID, text string) error {
	m.lastChannelID = channelID
	m.lastBody = text
	return m.sendErr
}

// defaultMockBot returns a mockBotSender configured for success.
func defaultMockBot() *mockBotSender {
	return &mockBotSender{}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_Code
// ---------------------------------------------------------------------------

func TestMezonAdapter_Code(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	got := adapter.Code()
	want := "mezon"
	if got != want {
		t.Errorf("Code() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_ChannelKey
// ---------------------------------------------------------------------------

func TestMezonAdapter_ChannelKey_DM(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	code, resID := adapter.ChannelKey([]byte(`{"channel_id": "dm_abc123"}`))
	if code != "mezon" {
		t.Errorf("provider code = %q, want %q", code, "mezon")
	}
	if resID != "dm_abc123" {
		t.Errorf("resource ID = %q, want %q", resID, "dm_abc123")
	}
}

func TestMezonAdapter_ChannelKey_Group(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	code, resID := adapter.ChannelKey([]byte(`{"channel_id": "ch_xyz789"}`))
	if code != "mezon" {
		t.Errorf("provider code = %q, want %q", code, "mezon")
	}
	if resID != "ch_xyz789" {
		t.Errorf("resource ID = %q, want %q", resID, "ch_xyz789")
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_ParseEvent
// ---------------------------------------------------------------------------

func TestMezonAdapter_ParseEvent(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	payload := []byte(`{
		"channel_id": "ch_abc",
		"sender_id": "user-789",
		"message_id": "msg-001",
		"text": "hello from mezon"
	}`)

	ev, err := adapter.ParseEvent(payload)
	if err != nil {
		t.Fatalf("ParseEvent() returned error: %v", err)
	}

	if ev.EventID != "msg-001" {
		t.Errorf("EventID = %q, want %q", ev.EventID, "msg-001")
	}
	if ev.EventType != "message.created" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "message.created")
	}
	if ev.Actor.ID != "user-789" {
		t.Errorf("Actor.ID = %q, want %q", ev.Actor.ID, "user-789")
	}
	if ev.Body != "hello from mezon" {
		t.Errorf("Body = %q, want %q", ev.Body, "hello from mezon")
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_WriteBack
// ---------------------------------------------------------------------------

func TestMezonAdapter_WriteBack_WebSocket(t *testing.T) {
	mock := defaultMockBot()
	adapter := NewMezonAdapter(mock)

	ctx := context.Background()
	err := adapter.WriteBack(ctx, "", "ch_abc", "Task complete")
	if err != nil {
		t.Fatalf("WriteBack() returned error: %v", err)
	}

	if mock.lastChannelID != "ch_abc" {
		t.Errorf("SendMessage channelID = %q, want %q", mock.lastChannelID, "ch_abc")
	}
	if mock.lastBody != "Task complete" {
		t.Errorf("SendMessage body = %q, want %q", mock.lastBody, "Task complete")
	}
}

func TestMezonAdapter_WriteBack_REST(t *testing.T) {
	mock := defaultMockBot()
	mock.sendErr = errors.New("websocket disconnected")
	adapter := NewMezonAdapter(mock)

	ctx := context.Background()
	err := adapter.WriteBack(ctx, "", "ch_fail", "oops")
	if err == nil {
		t.Fatal("WriteBack() expected error when SendMessage fails, got nil")
	}
	if mock.lastChannelID != "ch_fail" {
		t.Errorf("SendMessage channelID = %q, want %q", mock.lastChannelID, "ch_fail")
	}
	if mock.lastBody != "oops" {
		t.Errorf("SendMessage body = %q, want %q", mock.lastBody, "oops")
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_FetchTaskDetail
// ---------------------------------------------------------------------------

func TestMezonAdapter_FetchTaskDetail(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	ctx := context.Background()
	detail, err := adapter.FetchTaskDetail(ctx, "", "ch_abc")
	if err != nil {
		t.Fatalf("FetchTaskDetail() returned error: %v", err)
	}

	if detail == nil {
		t.Fatal("FetchTaskDetail() returned nil TaskDetail")
	}
	if detail.Title != "" {
		t.Errorf("Title = %q, want empty", detail.Title)
	}
	if detail.Description != "" {
		t.Errorf("Description = %q, want empty", detail.Description)
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_WebhookHeaders
// ---------------------------------------------------------------------------

func TestMezonAdapter_WebhookHeaders(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	headers := adapter.WebhookHeaders()
	if headers.Signature != "" {
		t.Errorf("Signature header = %q, want empty", headers.Signature)
	}
	if headers.Timestamp != "" {
		t.Errorf("Timestamp header = %q, want empty", headers.Timestamp)
	}
	if headers.DeliveryID != "" {
		t.Errorf("DeliveryID header = %q, want empty", headers.DeliveryID)
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_VerifyWebhook
// ---------------------------------------------------------------------------

func TestMezonAdapter_VerifyWebhook(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	err := adapter.VerifyWebhook([]byte(`{}`), "", "", "")
	if err != nil {
		t.Errorf("VerifyWebhook() expected nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_ExtractContainerID
// ---------------------------------------------------------------------------

func TestMezonAdapter_ExtractContainerID(t *testing.T) {
	bot := defaultMockBot()
	adapter := NewMezonAdapter(bot)

	containerID, err := adapter.ExtractContainerID([]byte(`{}`))
	if err != nil {
		t.Fatalf("ExtractContainerID() returned error: %v", err)
	}
	if containerID != "" {
		t.Errorf("ContainerID = %q, want empty", containerID)
	}
}

// ---------------------------------------------------------------------------
// TestMezonAdapter_RegisterAndLookup
// ---------------------------------------------------------------------------

// TestMezonAdapter_RegisterAndLookup registers the Mezon adapter via
// RegisterAdapter() without a bot argument (the new signature) and verifies
// the adapter is registered under the "mezon" code.
func TestMezonAdapter_RegisterAndLookup(t *testing.T) {
	// Save any previous adapter for cleanup.
	prev, _ := provider.Get("mezon")

	// Register the adapter without a bot argument (new signature).
	RegisterAdapter()
	t.Cleanup(func() {
		if prev != nil {
			provider.Register(prev)
		}
	})

	// Look up by code "mezon"
	got, ok := provider.Get("mezon")
	if !ok {
		t.Fatal("provider.Get(\"mezon\") returned ok=false")
	}
	if got.Code() != "mezon" {
		t.Errorf("adapter.Code() = %q, want %q", got.Code(), "mezon")
	}
}
