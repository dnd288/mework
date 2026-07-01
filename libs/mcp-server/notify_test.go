package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

// publishRecord captures a single bus publish event for later assertion.
type publishRecord struct {
	topic   string
	payload []byte
}

// fakeBus implements BusBroker for testing.
// It records every publish and keeps subscriber channels so that messages
// published after a subscription are delivered to the subscriber.
type fakeBus struct {
	mu        sync.Mutex
	published []publishRecord
	subs      map[string][]chan []byte
}

func newFakeBus() *fakeBus {
	return &fakeBus{
		subs: make(map[string][]chan []byte),
	}
}

func (f *fakeBus) Publish(_ context.Context, topic string, payload []byte) error {
	f.mu.Lock()
	f.published = append(f.published, publishRecord{topic: topic, payload: payload})
	// Deliver to existing matching subscribers.
	for _, ch := range f.subs[topic] {
		select {
		case ch <- payload:
		default:
		}
	}
	f.mu.Unlock()
	return nil
}

func (f *fakeBus) Subscribe(_ context.Context, topic string) (<-chan []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan []byte, 256)
	f.subs[topic] = append(f.subs[topic], ch)
	return ch, nil
}

// lastPublish returns the most recent publish record, if any.
func (f *fakeBus) lastPublish() (publishRecord, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.published) == 0 {
		return publishRecord{}, false
	}
	return f.published[len(f.published)-1], true
}

// publishCount returns how many publishes have been recorded.
func (f *fakeBus) publishCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.published)
}

// setupNotifyTest creates a fresh bus, handler, registry, and MCP test server
// for notify_human / ask_human tests. The caller must set MEWORK_SESSION_ID (or
// unset it) before calling this helper.
func setupNotifyTest(t *testing.T, bus BusBroker) *mcptest.Server {
	t.Helper()

	handler := NewNotifyHandler(bus) // UNDEFINED SYMBOL — RED failure
	if handler == nil {
		t.Fatal("NewNotifyHandler returned nil")
	}

	reg := NewToolRegistry()

	reg.Register("notify_human",
		mcp.NewTool("notify_human",
			mcp.WithDescription("Sends a notification to the human user"),
			mcp.WithString("message", mcp.Required(),
				mcp.Description("The message content to send")),
			mcp.WithString("format",
				mcp.Description("Message format: text or markdown")),
			mcp.WithArray("attachments",
				mcp.WithStringItems(mcp.Description("Optional file attachments"))),
		),
		handler.NotifyHuman, // UNDEFINED SYMBOL
	)

	reg.Register("ask_human",
		mcp.NewTool("ask_human",
			mcp.WithDescription("Asks the human a question and waits for a response"),
			mcp.WithString("question", mcp.Required(),
				mcp.Description("The question to ask the human")),
			mcp.WithArray("options",
				mcp.WithStringItems(mcp.Description("Valid response options"))),
			mcp.WithNumber("timeout_minutes",
				mcp.Description("Max time to wait for a response in minutes")),
		),
		handler.AskHuman, // UNDEFINED SYMBOL
	)

	tools := reg.ServerTools()
	srv, err := mcptest.NewServer(t, tools...)
	if err != nil {
		t.Fatalf("mcptest.NewServer: %v", err)
	}
	return srv
}

// TestNotifyAndAskTools exercises the notify_human and ask_human MCP tools.
//
// RED step: fails because NewNotifyHandler, NotifyHuman, and AskHuman
// (defined in notify.go) are not yet implemented. The test declares the
// expected API surface and scenarios drawn from delta-spec requirements.
func TestNotifyAndAskTools(t *testing.T) {
	// -----------------------------------------------------------------------
	// notify_human
	// -----------------------------------------------------------------------

	t.Run("notify_human publishes to session output", func(t *testing.T) {
		bus := newFakeBus()
		t.Setenv("MEWORK_SESSION_ID", "test-session")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()

		_, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "notify_human",
				Arguments: map[string]interface{}{
					"message": "hello",
				},
			},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}

		record, ok := bus.lastPublish()
		if !ok {
			t.Fatal("expected at least one bus publish")
		}
		if record.topic != "session.test-session.output" {
			t.Errorf("topic = %q, want %q", record.topic, "session.test-session.output")
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(record.payload, &msg); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if msg["message"] != "hello" {
			t.Errorf("payload message = %v, want %q", msg["message"], "hello")
		}
	})

	t.Run("notify_human without session ID logs to stdout", func(t *testing.T) {
		bus := newFakeBus()
		os.Unsetenv("MEWORK_SESSION_ID")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()

		_, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "notify_human",
				Arguments: map[string]interface{}{
					"message": "hello",
				},
			},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}

		if n := bus.publishCount(); n != 0 {
			t.Errorf("expected 0 bus publishes, got %d", n)
		}
	})

	t.Run("notify_human with format markdown sends structured message", func(t *testing.T) {
		bus := newFakeBus()
		t.Setenv("MEWORK_SESSION_ID", "test-session")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()

		_, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "notify_human",
				Arguments: map[string]interface{}{
					"message": "hello",
					"format":  "markdown",
				},
			},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}

		record, ok := bus.lastPublish()
		if !ok {
			t.Fatal("expected at least one bus publish")
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(record.payload, &msg); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if msg["format"] != "markdown" {
			t.Errorf("payload format = %v, want %q", msg["format"], "markdown")
		}
	})

	// -----------------------------------------------------------------------
	// ask_human
	// -----------------------------------------------------------------------

	t.Run("ask_human publishes and waits for response", func(t *testing.T) {
		bus := newFakeBus()
		t.Setenv("MEWORK_SESSION_ID", "test-session")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()
		client := srv.Client()

		// Subscribe before calling the tool so we receive the published question.
		respCh, err := bus.Subscribe(t.Context(), "session.test-session.output")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		// ask_human blocks until a response arrives — call it in a goroutine.
		type callResult struct {
			resp *mcp.CallToolResult
			err  error
		}
		resultCh := make(chan callResult, 1)
		go func() {
			r, e := client.CallTool(t.Context(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "ask_human",
					Arguments: map[string]interface{}{
						"question": "continue?",
						"options":  []interface{}{"yes", "no"},
					},
				},
			})
			resultCh <- callResult{resp: r, err: e}
		}()

		// Wait for the question to arrive on the bus, then respond.
		select {
		case <-respCh:
			respPayload, _ := json.Marshal(map[string]string{"response": "yes"})
			if err := bus.Publish(t.Context(), "session.test-session.input", respPayload); err != nil {
				t.Errorf("publish response: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for question publication on bus")
		}

		// Wait for ask_human to return.
		select {
		case r := <-resultCh:
			if r.err != nil {
				t.Fatalf("CallTool: %v", r.err)
			}
			if r.resp.IsError {
				t.Fatal("expected success, got isError=true")
			}
			if len(r.resp.Content) == 0 {
				t.Fatal("empty result content")
			}
			tc, ok := r.resp.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("expected TextContent, got %T", r.resp.Content[0])
			}
			var respData map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Text), &respData); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if respData["response"] != "yes" {
				t.Errorf("response = %v, want %q", respData["response"], "yes")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for ask_human result")
		}
	})

	t.Run("ask_human times out", func(t *testing.T) {
		bus := newFakeBus()
		t.Setenv("MEWORK_SESSION_ID", "test-session")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()

		// 0.01 minutes = 600 ms — the handler should time out before the
		// overall test timeout fires.
		_, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "ask_human",
				Arguments: map[string]interface{}{
					"question":       "continue?",
					"timeout_minutes": 0.01,
				},
			},
		})
		if err == nil {
			t.Error("expected timeout error, got nil")
		}
	})

	t.Run("ask_human without session ID returns error", func(t *testing.T) {
		bus := newFakeBus()
		os.Unsetenv("MEWORK_SESSION_ID")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()

		_, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "ask_human",
				Arguments: map[string]interface{}{
					"question": "continue?",
				},
			},
		})
		if err == nil {
			t.Error("expected error for missing session ID, got nil")
		}
	})

	t.Run("ask_human with options validates response", func(t *testing.T) {
		bus := newFakeBus()
		t.Setenv("MEWORK_SESSION_ID", "test-session")
		srv := setupNotifyTest(t, bus)
		defer srv.Close()
		client := srv.Client()

		// Subscribe before calling the tool so we catch the question.
		respCh, err := bus.Subscribe(t.Context(), "session.test-session.output")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		// ask_human blocks — call it in a goroutine.
		type callResult struct {
			resp *mcp.CallToolResult
			err  error
		}
		resultCh := make(chan callResult, 1)
		go func() {
			r, e := client.CallTool(t.Context(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "ask_human",
					Arguments: map[string]interface{}{
						"question": "continue?",
						"options":  []interface{}{"yes", "no"},
					},
				},
			})
			resultCh <- callResult{resp: r, err: e}
		}()

		// Wait for the question, then send an INVALID response first.
		select {
		case <-respCh:
			invalidPayload, _ := json.Marshal(map[string]string{"response": "maybe"})
			if err := bus.Publish(t.Context(), "session.test-session.input", invalidPayload); err != nil {
				t.Errorf("publish invalid response: %v", err)
			}

			// Brief pause so the handler has time to reject the invalid one.
			// Retry-based wait below — see waitForHandler helper

			// Now publish a VALID response.
			validPayload, _ := json.Marshal(map[string]string{"response": "yes"})
			if err := bus.Publish(t.Context(), "session.test-session.input", validPayload); err != nil {
				t.Errorf("publish valid response: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for question publication")
		}

		// The result should be "yes" (the valid option), not "maybe".
		select {
		case r := <-resultCh:
			if r.err != nil {
				t.Fatalf("CallTool: %v", r.err)
			}
			if r.resp.IsError {
				t.Fatal("expected success, got isError=true")
			}
			if len(r.resp.Content) == 0 {
				t.Fatal("empty result content")
			}
			tc, ok := r.resp.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("expected TextContent, got %T", r.resp.Content[0])
			}
			var respData map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Text), &respData); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if respData["response"] != "yes" {
				t.Errorf("response = %v, want %q", respData["response"], "yes")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for ask_human result")
		}
	})
}
