package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/libs/client/subscribe"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/shared/transport"
)

func TestEngine_SubscribesOnStart(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	var claimCount atomic.Int32

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-rt-1", accountID: "test-account-1"}.Middleware)

	// Claim endpoint (old behavior) — must NOT be called.
	mux.Post("/api/v1/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		claimCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	// Subscribe endpoint (new behavior).
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	eng := NewEngine("test-runner", "test-secret", server.URL, "http://catalog.local")
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer eng.Stop()

	// Let the engine establish the SSE subscription.
	time.Sleep(300 * time.Millisecond)

	if claimCount.Load() > 0 {
		t.Error("engine should use Subscribe instead of Claim; Claim was called")
	}

	if eng.getStream() == nil {
		t.Error("engine should have an active SSE stream after Start")
	}
}

func TestEngine_ReceivesDispatchByPush(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-runner", accountID: "test-account-1"}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	eng := NewEngine("test-runner", "test-secret", server.URL, "http://catalog.local")
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

	// Hook to intercept the dispatch without running the full lifecycle.
	dispatchReceived := make(chan transport.Dispatch, 1)
	eng.dispatchHook = func(d transport.Dispatch, eventID string) {
		dispatchReceived <- d
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer eng.Stop()

	time.Sleep(200 * time.Millisecond)

	// Publish a dispatch on the runner's topic.
	d := transport.Dispatch{
		Agent:   transport.AgentRef{Name: "code-fixer", Version: "1.2.0"},
		Grant:   json.RawMessage(`{}`),
		Session: "sess-001",
		Runner:  "test-runner",
	}
	topic := bus.FormatTopic(bus.TopicRunnerDispatch, "test-runner")

	if err := broker.Publish(context.Background(), topic, bus.Message{
		Payload: mustMarshal(t, d),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Verify the engine received the dispatch by push (no poll request).
	select {
	case received := <-dispatchReceived:
		if received.Agent.Name != "code-fixer" || received.Agent.Version != "1.2.0" {
			t.Errorf("unexpected dispatch: %+v", received)
		}
		if received.Session != "sess-001" {
			t.Errorf("unexpected session: %s", received.Session)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not receive dispatch within 3 seconds")
	}
}

func TestEngine_ReconnectOnDisconnect(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-runner", accountID: "test-account-1"}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	eng := NewEngine("test-runner", "test-secret", server.URL, "http://catalog.local")
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer eng.Stop()

	time.Sleep(200 * time.Millisecond)

	firstStream := eng.getStream()
	if firstStream == nil {
		t.Fatal("engine should have an active stream after Start")
	}

	// Close the current stream to simulate a disconnection.
	firstStream.Close()

	// Wait for the engine to reconnect (backoff starts at ~1s, jitter added).
	var newStream *subscribe.SSEStream
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for newStream == nil {
		select {
		case <-deadline:
			t.Fatal("engine did not reconnect within 10 seconds")
		case <-ticker.C:
			if s := eng.getStream(); s != nil && s != firstStream {
				newStream = s
			}
		}
	}

	if newStream == nil {
		t.Fatal("engine should have created a new SSE stream after reconnect")
	}
	if newStream == firstStream {
		t.Error("new stream should be a different instance from the old one")
	}
}

func TestEngine_ProcessesOneDispatchAtATime(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-runner", accountID: "test-account-1"}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	eng := NewEngine("test-runner", "test-secret", server.URL, "http://catalog.local")
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

	// processing tracks whether a dispatch is currently being handled.
	var mu sync.Mutex
	processing := false
	started := make(chan struct{}, 2)
	finished := make(chan struct{}, 2)
	blocker := make(chan struct{})

	eng.dispatchHook = func(d transport.Dispatch, eventID string) {
		mu.Lock()
		processing = true
		mu.Unlock()

		started <- struct{}{}

		// Block until the test unblocks us.
		<-blocker

		mu.Lock()
		processing = false
		mu.Unlock()
		finished <- struct{}{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer eng.Stop()

	time.Sleep(200 * time.Millisecond)

	topic := bus.FormatTopic(bus.TopicRunnerDispatch, "test-runner")
	grantJSON := json.RawMessage(`{}`)

	// Publish first dispatch — will be processed and block.
	if err := broker.Publish(context.Background(), topic, bus.Message{
		Payload: mustMarshal(t, transport.Dispatch{
			Agent:   transport.AgentRef{Name: "agent-a", Version: "1.0"},
			Grant:   grantJSON,
			Session: "sess-a",
			Runner:  "test-runner",
		}),
	}); err != nil {
		t.Fatalf("publish session-a: %v", err)
	}

	// Wait for the first dispatch to start processing.
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first dispatch was not picked up within 3 seconds")
	}

	mu.Lock()
	if !processing {
		mu.Unlock()
		t.Fatal("first dispatch should be processing")
	}
	mu.Unlock()

	// Publish second dispatch while first is still blocking.
	if err := broker.Publish(context.Background(), topic, bus.Message{
		Payload: mustMarshal(t, transport.Dispatch{
			Agent:   transport.AgentRef{Name: "agent-b", Version: "2.0"},
			Grant:   grantJSON,
			Session: "sess-b",
			Runner:  "test-runner",
		}),
	}); err != nil {
		t.Fatalf("publish session-b: %v", err)
	}

	// Give the second dispatch time to be delivered to the internal channel.
	time.Sleep(500 * time.Millisecond)

	// Verify only one dispatch is being processed (the second is queued).
	mu.Lock()
	if !processing {
		mu.Unlock()
		t.Fatal("only one dispatch should be processing at a time; second is queued")
	}
	mu.Unlock()

	// Unblock the first dispatch so the second can proceed.
	close(blocker)

	// Both dispatches should complete.
	for i := 0; i < 2; i++ {
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Fatalf("dispatch %d did not complete within 3 seconds", i+1)
		}
	}
}

func TestEngine_DeliversConcurrentDispatches(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-runner", accountID: "test-account-1"}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	eng := NewEngine("test-runner", "test-secret", server.URL, "http://catalog.local")
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

	var receivedMu sync.Mutex
	received := make([]string, 0, 3)
	var receivedWg sync.WaitGroup
	receivedWg.Add(3)

	eng.dispatchHook = func(d transport.Dispatch, eventID string) {
		receivedMu.Lock()
		received = append(received, d.Session)
		receivedMu.Unlock()
		receivedWg.Done()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer eng.Stop()

	time.Sleep(200 * time.Millisecond)

	topic := bus.FormatTopic(bus.TopicRunnerDispatch, "test-runner")
	grantJSON := json.RawMessage(`{}`)

	// Publish three dispatches rapidly (they all land in the buffered channel).
	for _, s := range []string{"sess-a", "sess-b", "sess-c"} {
		if err := broker.Publish(context.Background(), topic, bus.Message{
			Payload: mustMarshal(t, transport.Dispatch{
				Agent:   transport.AgentRef{Name: "agent", Version: "1.0"},
				Grant:   grantJSON,
				Session: s,
				Runner:  "test-runner",
			}),
		}); err != nil {
			t.Fatalf("publish %s: %v", s, err)
		}
	}

	// All three dispatches should arrive.
	c := make(chan struct{})
	go func() {
		receivedWg.Wait()
		close(c)
	}()

	select {
	case <-c:
		// All three received.
	case <-time.After(5 * time.Second):
		t.Fatal("not all 3 dispatches were delivered within 5 seconds")
	}

	receivedMu.Lock()
	if len(received) != 3 {
		t.Fatalf("expected 3 dispatches delivered, got %d", len(received))
	}
	sessions := make(map[string]bool)
	for _, s := range received {
		sessions[s] = true
	}
	if len(sessions) != 3 {
		t.Error("duplicate sessions detected; all three should be unique")
	}
	receivedMu.Unlock()
}

// mustMarshal marshals v to JSON, fatal on error.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
