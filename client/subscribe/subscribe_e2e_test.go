package subscribe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/client/subscribe"
	"mework/server/bus"
	"mework/server/bus/memory"
	"mework/server/middleware"
)

// testRuntimeInjector sets the runtime_id context value for test requests.
type testRuntimeInjector struct{}

func (testRuntimeInjector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.RuntimeIDKey, "test-rt-1")
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func TestSSEConsumerE2E(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)
	ackHandler := bus.NewAckHandler(broker)

	mux := chi.NewRouter()
	mux.Use(testRuntimeInjector{}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)
	mux.Post("/api/v1/jobs/messages/{msgID}/ack", ackHandler.Ack)

	server := httptest.NewServer(mux)
	defer server.Close()

	client := subscribe.NewClient(server.URL, 5*time.Second)

	t.Run("subscribe receives pushed message", func(t *testing.T) {
		// RED: client.Subscribe does not exist yet.
		stream, err := client.Subscribe("rt-token", []string{"runner.test.dispatch"}, "")
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		defer stream.Close()

		ctx := context.Background()
		err = broker.Publish(ctx, bus.Topic("runner.test.dispatch"), bus.Message{
			Payload: []byte(`{"id":"job-1","instructions":"test"}`),
		})
		if err != nil {
			t.Fatalf("publish: %v", err)
		}

		select {
		case ev, ok := <-stream.Events():
			if !ok {
				t.Fatal("stream closed unexpectedly")
			}
			if ev.ID == "" {
				t.Error("expected non-empty event ID")
			}
			if ev.Topic != bus.Topic("runner.test.dispatch") {
				t.Errorf("expected topic runner.test.dispatch, got %s", ev.Topic)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for SSE event")
		}
	})

	t.Run("last-event-id resume", func(t *testing.T) {
		// RED: SSEStream type and Subscribe method do not exist yet.
		stream, err := client.Subscribe("rt-token", []string{"runner.test.dispatch"}, "")
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		defer stream.Close()

		// Read first event.
		select {
		case ev, ok := <-stream.Events():
			if !ok {
				t.Fatal("stream closed")
			}
			// Disconnect by closing the stream.
			stream.Close()
			_ = ev

			// Re-subscribe with Last-Event-ID.
			stream2, err := client.Subscribe("rt-token", []string{"runner.test.dispatch"}, ev.ID)
			if err != nil {
				t.Fatalf("re-subscribe: %v", err)
			}
			defer stream2.Close()

			// Should receive only messages newer than ev.ID — no duplicates.
			select {
			case dup, ok := <-stream2.Events():
				if ok {
					t.Errorf("expected no duplicate delivery after resume with Last-Event-ID, got event %s", dup.ID)
				}
			default:
				// No immediate event — correct: no new messages.
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for first event")
		}
	})

	t.Run("post ack after terminal handling", func(t *testing.T) {
		// RED: client.AckMessage does not exist yet.
		stream, err := client.Subscribe("rt-token", []string{"runner.test.dispatch"}, "")
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		defer stream.Close()

		ctx := context.Background()
		err = broker.Publish(ctx, bus.Topic("runner.test.dispatch"), bus.Message{
			Payload: []byte(`{"id":"job-2"}`),
		})
		if err != nil {
			t.Fatalf("publish: %v", err)
		}

		select {
		case ev, ok := <-stream.Events():
			if !ok {
				t.Fatal("stream closed")
			}
			// Simulate terminal handling and POST ack.
			err := client.AckMessage("rt-token", ev.ID)
			if err != nil {
				t.Fatalf("ack message: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for event to ack")
		}
	})
}
