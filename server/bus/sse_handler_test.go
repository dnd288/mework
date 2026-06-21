package bus_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/server/bus"
	"mework/server/bus/memory"
	"mework/server/middleware"
)

// testAuthMiddleware simulates the rt_token runtime authenticator by setting
// runtime identity in the request context.
func testAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.RuntimeIDKey, "test-runtime")
		ctx = context.WithValue(ctx, middleware.AccountIDKey, "test-account")
		ctx = context.WithValue(ctx, middleware.TenantIDKey, "test-tenant")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// noAuthMiddleware simulates an unauthenticated request — used for negative tests.
func noAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized: missing runtime token", http.StatusUnauthorized)
	})
}

// sseEvent holds a single parsed SSE event.
type sseEvent struct {
	ID    string
	Event string
	Data  string
}

// readSSEEvent reads one SSE event from a buffered reader. The reader should
// be backed by an SSE response body. Returns io.EOF when the stream ends.
func readSSEEvent(r *bufio.Reader) (sseEvent, error) {
	var ev sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return ev, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// Blank line terminates an SSE event.
			return ev, nil
		}
		if strings.HasPrefix(line, ":") {
			// Comment line — skip but don't terminate.
			continue
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			ev.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			ev.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.Data += strings.TrimPrefix(line, "data: ")
		}
	}
}

// TestSSEHandler_Subscribe verifies the SSE subscription handler against the
// message-bus delta-spec scenarios.
func TestSSEHandler_Subscribe(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "receive a pushed event BUS-03",
			run: func(t *testing.T) {
				broker := memory.New()
				h := bus.NewSSEHandler(broker)

				r := chi.NewRouter()
				r.Use(testAuthMiddleware)
				r.Get("/subscribe", h.Subscribe)

				ts := httptest.NewServer(r)
				defer ts.Close()

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/subscribe?topics=runner.R.dispatch", nil)
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
				if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
					t.Errorf("expected Content-Type text/event-stream, got %q", ct)
				}

				// Publish a message while the SSE stream is open.
				if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("hello")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}

				br := bufio.NewReader(resp.Body)
				ev, err := readSSEEvent(br)
				if err != nil {
					t.Fatalf("readSSEEvent: %v", err)
				}
				if ev.ID == "" {
					t.Error("expected non-empty event id")
				}
				if ev.Event != "runner.R.dispatch" {
					t.Errorf("expected event topic %q, got %q", "runner.R.dispatch", ev.Event)
				}
				if ev.Data != "hello" {
					t.Errorf("expected data %q, got %q", "hello", ev.Data)
				}
			},
		},
		{
			name: "multiple topics one stream BUS-04",
			run: func(t *testing.T) {
				broker := memory.New()
				h := bus.NewSSEHandler(broker)

				r := chi.NewRouter()
				r.Use(testAuthMiddleware)
				r.Get("/subscribe", h.Subscribe)

				ts := httptest.NewServer(r)
				defer ts.Close()

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/subscribe?topics=topic.A,topic.B", nil)
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

				// Publish to both topics while the SSE stream is open.
				if err := broker.Publish(ctx, bus.Topic("topic.A"), bus.Message{Payload: []byte("a")}); err != nil {
					t.Fatalf("Publish A: %v", err)
				}
				if err := broker.Publish(ctx, bus.Topic("topic.B"), bus.Message{Payload: []byte("b")}); err != nil {
					t.Fatalf("Publish B: %v", err)
				}

				// Read both events.
				br := bufio.NewReader(resp.Body)
				seen := make(map[string]bool)
				for i := 0; i < 2; i++ {
					ev, err := readSSEEvent(br)
					if err != nil {
						t.Fatalf("readSSEEvent %d: %v", i, err)
					}
					seen[ev.Event] = true
				}
				if !seen["topic.A"] || !seen["topic.B"] {
					t.Errorf("expected both topics, got %v", seen)
				}
			},
		},
		{
			name: "resume with Last-Event-ID BUS-05",
			run: func(t *testing.T) {
				broker := memory.New()
				ctx := context.Background()

				// Publish two messages before any SSE subscription.
				if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("first")}); err != nil {
					t.Fatalf("Publish first: %v", err)
				}
				if err := broker.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("second")}); err != nil {
					t.Fatalf("Publish second: %v", err)
				}

				// Determine the first message's ID by subscribing directly.
				sub, err := broker.Subscribe(ctx, bus.Identity("lookup"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe lookup: %v", err)
				}
				select {
				case ev := <-sub.Events():
					// Open an SSE connection with last_event_id=first-msg-id.
					h := bus.NewSSEHandler(broker)
					r := chi.NewRouter()
					r.Use(testAuthMiddleware)
					r.Get("/subscribe", h.Subscribe)
					ts := httptest.NewServer(r)
					defer ts.Close()

					subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					req, err := http.NewRequestWithContext(subCtx, "GET",
						ts.URL+"/subscribe?topics=runner.R.dispatch&last_event_id="+ev.ID, nil)
					if err != nil {
						t.Fatalf("NewRequest: %v", err)
					}
					req.Header.Set("Authorization", "Bearer test")

					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET subscribe: %v", err)
					}
					defer resp.Body.Close()

					// The broker should replay the second message (id > first's id)
					br := bufio.NewReader(resp.Body)
					gotEv, err := readSSEEvent(br)
					if err != nil {
						t.Fatalf("readSSEEvent: %v", err)
					}
					if gotEv.Data != "second" {
						t.Errorf("expected payload %q, got %q", "second", gotEv.Data)
					}

				case <-time.After(time.Second):
					t.Fatal("timed out reading first message from broker")
				}
				sub.Close()
			},
		},
		{
			name: "per-topic ordering CONC-04",
			run: func(t *testing.T) {
				broker := memory.New()
				ctx := context.Background()
				h := bus.NewSSEHandler(broker)

				r := chi.NewRouter()
				r.Use(testAuthMiddleware)
				r.Get("/subscribe", h.Subscribe)
				ts := httptest.NewServer(r)
				defer ts.Close()

				subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(subCtx, "GET", ts.URL+"/subscribe?topics=ordered.topic", nil)
				if err != nil {
					t.Fatalf("NewRequest: %v", err)
				}
				req.Header.Set("Authorization", "Bearer test")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET subscribe: %v", err)
				}
				defer resp.Body.Close()

				// Publish three messages in a known order.
				for _, payload := range []string{"first", "second", "third"} {
					if err := broker.Publish(ctx, bus.Topic("ordered.topic"), bus.Message{Payload: []byte(payload)}); err != nil {
						t.Fatalf("Publish %s: %v", payload, err)
					}
				}

				br := bufio.NewReader(resp.Body)
				var received []string
				for len(received) < 3 {
					ev, err := readSSEEvent(br)
					if err != nil {
						t.Fatalf("readSSEEvent after %d events: %v", len(received), err)
					}
					received = append(received, ev.Data)
				}
				if len(received) != 3 {
					t.Fatalf("expected 3 events, got %d", len(received))
				}
				if received[0] != "first" || received[1] != "second" || received[2] != "third" {
					t.Errorf("per-topic ordering violated: got %v, want [first second third]", received)
				}
			},
		},
		{
			name: "slow subscriber backpressure BUS-16",
			run: func(t *testing.T) {
				broker := memory.New()
				ctx := context.Background()
				h := bus.NewSSEHandler(broker)

				r := chi.NewRouter()
				r.Use(testAuthMiddleware)
				r.Get("/subscribe", h.Subscribe)
				ts := httptest.NewServer(r)
				defer ts.Close()

				subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(subCtx, "GET", ts.URL+"/subscribe?topics=busy.topic", nil)
				if err != nil {
					t.Fatalf("NewRequest: %v", err)
				}
				req.Header.Set("Authorization", "Bearer test")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET subscribe: %v", err)
				}
				defer resp.Body.Close()

				// Publish more messages than a small per-subscriber buffer.
				// Each publish should not block (the handler drops oldest
				// for a slow subscriber).
				const count = 300
				publishStart := time.Now()
				for i := 0; i < count; i++ {
					if err := broker.Publish(ctx, bus.Topic("busy.topic"),
						bus.Message{Payload: []byte(fmt.Sprintf("msg-%d", i))}); err != nil {
						t.Fatalf("Publish %d: %v", i, err)
					}
				}
				publishDuration := time.Since(publishStart)

				// Reading after all publishes — the handler should not have blocked
				// and should have delivered a subset (some may have been dropped).
				br := bufio.NewReader(resp.Body)
				var readCount int
				readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
				defer readCancel()
				for {
					select {
					case <-readCtx.Done():
						goto done
					default:
					}
					ev, err := readSSEEvent(br)
					if err != nil {
						break
					}
					_ = ev
					readCount++
				}
			done:
				if publishDuration > 2*time.Second {
					t.Errorf("publishing %d messages took %v, suggesting blocking", count, publishDuration)
				}
				if readCount == 0 {
					t.Error("expected at least some events to be delivered")
				}
				t.Logf("published %d, read %d events in %v", count, readCount, publishDuration)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

// TestSSEHandler_Heartbeat verifies that the SSE handler sends periodic comment
// lines to keep the connection alive.
func TestSSEHandler_Heartbeat(t *testing.T) {
	broker := memory.New()
	h := bus.NewSSEHandler(broker, bus.WithHeartbeatInterval(100*time.Millisecond))

	r := chi.NewRouter()
	r.Use(testAuthMiddleware)
	r.Get("/subscribe", h.Subscribe)

	ts := httptest.NewServer(r)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/subscribe?topics=test.topic", nil)
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

	// Read from the response body and look for heartbeat comment lines.
	br := bufio.NewReader(resp.Body)
	deadline := time.After(500 * time.Millisecond)
	ok := false
	for {
		select {
		case <-deadline:
			if !ok {
				t.Fatal("did not receive heartbeat comment within deadline")
			}
			return
		default:
		}
		line, err := br.ReadString('\n')
		if err != nil {
			if !ok {
				t.Fatalf("read error before heartbeat: %v", err)
			}
			return
		}
		if strings.HasPrefix(line, ": heartbeat") {
			ok = true
			return
		}
	}
}
