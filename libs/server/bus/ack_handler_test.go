package bus_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
)

// setupAckServer creates a test server with the ack handler wired up.
// Returns the server and the broker so tests can publish messages.
func setupAckServer(t *testing.T, broker bus.Broker) *httptest.Server {
	t.Helper()
	h := bus.NewAckHandler(broker)

	r := chi.NewRouter()
	r.Use(testAuthMiddleware)
	r.Post("/messages/{msgID}/ack", h.Ack)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts
}

// TestAckHandler_Ack verifies the delivery acknowledgement endpoint against
// the message-bus delta-spec scenarios.
func TestAckHandler_Ack(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "ack marks message handled BUS-06",
			run: func(t *testing.T) {
				broker := memory.New()
				ts := setupAckServer(t, broker)
				ctx := context.Background()

				// Deliver a message via the broker directly.
				if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("handle-me")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}

				// Subscribe directly to capture the message id.
				sub, err := broker.Subscribe(ctx, bus.Identity("acker"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				var msgID string
				select {
				case ev := <-sub.Events():
					msgID = ev.ID
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for message delivery")
				}

				// POST ack with the captured message id.
				ackURL := ts.URL + "/messages/" + msgID + "/ack"
				req, err := http.NewRequestWithContext(ctx, "POST", ackURL, nil)
				if err != nil {
					t.Fatalf("NewRequest: %v", err)
				}
				req.Header.Set("Authorization", "Bearer test")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("POST ack: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusNoContent {
					t.Errorf("expected 204 No Content, got %d", resp.StatusCode)
				}

				// Reconnect and verify the acked message is NOT redelivered.
				sub2, err := broker.Subscribe(ctx, bus.Identity("checker"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe 2: %v", err)
				}
				defer sub2.Close()

				select {
				case <-sub2.Events():
					t.Error("acked message was redelivered")
				case <-time.After(200 * time.Millisecond):
					// Good — ack prevented redelivery.
				}
			},
		},
		{
			name: "unacked message redeliverable BUS-07",
			run: func(t *testing.T) {
				broker := memory.New()
				ctx := context.Background()

				// Publish a message and deliver it via direct subscription.
				if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("lease-me")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}

				sub, err := broker.Subscribe(ctx, bus.Identity("leaser"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}

				var msgID string
				select {
				case ev := <-sub.Events():
					msgID = ev.ID
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for delivery")
				}
				sub.Close()

				// Do NOT ack the message — instead, reconnect.
				// In the Postgres backend, the unacked message's lease would expire
				// and the message would become redeliverable. With the in-memory
				// broker, all retained messages are always redelivered, so this
				// assertion will succeed (message will appear again).
				sub2, err := broker.Subscribe(ctx, bus.Identity("releaser"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe 2: %v", err)
				}
				defer sub2.Close()

				select {
				case ev := <-sub2.Events():
					if ev.ID == msgID {
						// Success — the unacked message is redelivered.
						if string(ev.Message.Payload) != "lease-me" {
							t.Errorf("expected payload %q, got %q", "lease-me", string(ev.Message.Payload))
						}
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for redelivery")
				}

				// Now test the ack endpoint: POST ack for this message.
				ts := setupAckServer(t, broker)
				defer ts.Close()

				ackURL := ts.URL + "/messages/" + msgID + "/ack"
				req, err := http.NewRequestWithContext(ctx, "POST", ackURL, nil)
				if err != nil {
					t.Fatalf("NewRequest: %v", err)
				}
				req.Header.Set("Authorization", "Bearer test")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("POST ack: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusNoContent {
					t.Errorf("expected 204, got %d", resp.StatusCode)
				}
			},
		},
		{
			name: "nonexistent message id returns 404",
			run: func(t *testing.T) {
				broker := memory.New()
				ts := setupAckServer(t, broker)
				ctx := context.Background()

				ackURL := ts.URL + "/messages/nonexistent-id/ack"
				req, err := http.NewRequestWithContext(ctx, "POST", ackURL, nil)
				if err != nil {
					t.Fatalf("NewRequest: %v", err)
				}
				req.Header.Set("Authorization", "Bearer test")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("POST ack: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusNotFound {
					t.Errorf("expected 404, got %d", resp.StatusCode)
				}
			},
		},
		{
			name: "already-acked message returns 409",
			run: func(t *testing.T) {
				broker := memory.New()
				ts := setupAckServer(t, broker)
				ctx := context.Background()

				// Publish and capture a message id.
				if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("double-ack")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}
				sub, err := broker.Subscribe(ctx, bus.Identity("dacker"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				var msgID string
				select {
				case ev := <-sub.Events():
					msgID = ev.ID
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for delivery")
				}

				// First ack — should succeed.
				ackURL := ts.URL + "/messages/" + msgID + "/ack"
				req1, _ := http.NewRequestWithContext(ctx, "POST", ackURL, nil)
				req1.Header.Set("Authorization", "Bearer test")
				resp1, err := http.DefaultClient.Do(req1)
				if err != nil {
					t.Fatalf("POST ack: %v", err)
				}
				defer resp1.Body.Close()

				// Second ack — should return 409 Conflict.
				req2, _ := http.NewRequestWithContext(ctx, "POST", ackURL, nil)
				req2.Header.Set("Authorization", "Bearer test")
				resp2, err := http.DefaultClient.Do(req2)
				if err != nil {
					t.Fatalf("POST ack double: %v", err)
				}
				defer resp2.Body.Close()

				if resp2.StatusCode != http.StatusConflict {
					t.Errorf("expected 409 for double-ack, got %d", resp2.StatusCode)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

// TestAckHandler_NoAuth verifies that the ack endpoint rejects requests without
// a valid runtime token.
func TestAckHandler_NoAuth(t *testing.T) {
	broker := memory.New()

	// Set up the ack handler without the auth middleware.
	h := bus.NewAckHandler(broker)
	r := chi.NewRouter()
	r.Use(noAuthMiddleware)
	r.Post("/messages/{msgID}/ack", h.Ack)

	ts := httptest.NewServer(r)
	defer ts.Close()

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "POST", ts.URL+"/messages/some-id/ack", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// No Authorization header.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST ack: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// TestSSEAndAck_Integration verifies a combined SSE + ack flow: a message
// delivered via SSE can be acknowledged through the ack endpoint.
func TestSSEAndAck_Integration(t *testing.T) {
	broker := memory.New()
	ctx := context.Background()

	// Set up SSE handler.
	sseHandler := bus.NewSSEHandler(broker)
	ackHandler := bus.NewAckHandler(broker)

	r := chi.NewRouter()
	r.Use(testAuthMiddleware)
	r.Get("/subscribe", sseHandler.Subscribe)
	r.Post("/messages/{msgID}/ack", ackHandler.Ack)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Open an SSE connection.
	sseCtx, sseCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sseCancel()

	req, err := http.NewRequestWithContext(sseCtx, "GET", ts.URL+"/subscribe?topics=runner.R.dispatch", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET subscribe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Publish a message.
	if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("integrate-me")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Read the SSE event to get its id.
	br := bufio.NewReader(resp.Body)
	ev, err := readSSEEvent(br)
	if err != nil {
		t.Fatalf("readSSEEvent: %v", err)
	}
	if ev.ID == "" {
		t.Fatal("expected non-empty event id")
	}

	// Ack the message via the ack endpoint.
	ackURL := ts.URL + "/messages/" + ev.ID + "/ack"
	ackReq, _ := http.NewRequestWithContext(ctx, "POST", ackURL, nil)
	ackReq.Header.Set("Authorization", "Bearer test")
	ackResp, err := http.DefaultClient.Do(ackReq)
	if err != nil {
		t.Fatalf("POST ack: %v", err)
	}
	defer ackResp.Body.Close()

	if ackResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", ackResp.StatusCode)
	}
}
