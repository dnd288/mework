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
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	"mework/libs/shared/transport"
)

func TestDispatch_SuccessfulLifecycle(t *testing.T) {
	// --- Setup ---
	secret := "test-runner-secret-key-32bytes!!"
	key := []byte(secret)

	t.Logf("Creating signed grant with ops: pull, spawn")
	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, key)
	if err != nil {
		t.Fatal(err)
	}
	rawGrant, _ := json.Marshal(g)

	// Track HTTP calls.
	var pullCalled atomic.Bool
	var resultStatus atomic.Value // holds string
	var ackCalled atomic.Bool

	// Mock catalog server: handles agent pull.
	catalogMux := http.NewServeMux()
	catalogMux.HandleFunc("/api/v1/agents/code-fixer/versions/1.2.0/pull", func(w http.ResponseWriter, r *http.Request) {
		pullCalled.Store(true)
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-Dispatch-Grant") == "" {
			t.Error("expected X-Dispatch-Grant header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(transport.Artifact{
			Ref:     transport.AgentRef{Name: "code-fixer", Version: "1.2.0"},
			Form:    transport.FormDefinition,
			Content: []byte("fix the code"),
		})
	})
	catalogSrv := httptest.NewServer(catalogMux)
	defer catalogSrv.Close()

	// Mock hub server: handles result POST and ack.
	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/api/v1/runners/sessions/sess-001/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		resultStatus.Store(body["status"])
		w.WriteHeader(http.StatusOK)
	})
	hubMux.HandleFunc("/api/v1/jobs/messages/evt-001/ack", func(w http.ResponseWriter, r *http.Request) {
		ackCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	hubSrv := httptest.NewServer(hubMux)
	defer hubSrv.Close()

	// Override runAgent to return success (no real AI CLI needed).
	origRunAgent := runAgent
	runAgent = func(ctx context.Context, artifact *transport.Artifact) core.Result {
		return core.Result{Output: "mock success", ExitCode: 0}
	}
	defer func() { runAgent = origRunAgent }()

	dispatch := transport.Dispatch{
		Agent:   transport.AgentRef{Name: "code-fixer", Version: "1.2.0"},
		Grant:   rawGrant,
		Session: "sess-001",
		Runner:  "test-runner",
	}

	client := subscribe.NewClient(hubSrv.URL, 10*time.Second)
	opts := processOpts{
		hubURL:     hubSrv.URL,
		catalogURL: catalogSrv.URL,
		secret:     secret,
		client:     client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Execute ---
	err = processDispatch(ctx, dispatch, "evt-001", opts)
	if err != nil {
		t.Fatalf("processDispatch failed: %v", err)
	}

	// --- Verify ---
	if !pullCalled.Load() {
		t.Error("catalog pull was not called")
	}
	actualStatus, _ := resultStatus.Load().(string)
	if actualStatus != "done" {
		t.Errorf("expected result status 'done', got %q", actualStatus)
	}
	if !ackCalled.Load() {
		t.Error("ack was not called")
	}
}

func TestDispatch_FailedRun(t *testing.T) {
	secret := "test-runner-secret-key-32bytes!!"
	key := []byte(secret)

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, key)
	if err != nil {
		t.Fatal(err)
	}
	rawGrant, _ := json.Marshal(g)

	// Mock catalog.
	catalogMux := http.NewServeMux()
	catalogMux.HandleFunc("/api/v1/agents/code-fixer/versions/1.2.0/pull", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(transport.Artifact{
			Ref:     transport.AgentRef{Name: "code-fixer", Version: "1.2.0"},
			Form:    transport.FormDefinition,
			Content: []byte("fix the code"),
		})
	})
	catalogSrv := httptest.NewServer(catalogMux)
	defer catalogSrv.Close()

	var resultStatus atomic.Value
	var ackCalled atomic.Bool

	hubMux := http.NewServeMux()
	hubMux.HandleFunc("/api/v1/runners/sessions/sess-002/result", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		resultStatus.Store(body["status"])
		if body["error"] == "" {
			t.Error("expected non-empty error in failed result")
		}
		w.WriteHeader(http.StatusOK)
	})
	hubMux.HandleFunc("/api/v1/jobs/messages/evt-002/ack", func(w http.ResponseWriter, r *http.Request) {
		ackCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	hubSrv := httptest.NewServer(hubMux)
	defer hubSrv.Close()

	// Override runAgent to return failure.
	origRunAgent := runAgent
	runAgent = func(ctx context.Context, artifact *transport.Artifact) core.Result {
		return core.Result{
			Error:    "agent crashed: OOM",
			ExitCode: 137,
		}
	}
	defer func() { runAgent = origRunAgent }()

	dispatch := transport.Dispatch{
		Agent:   transport.AgentRef{Name: "code-fixer", Version: "1.2.0"},
		Grant:   rawGrant,
		Session: "sess-002",
		Runner:  "test-runner",
	}

	client := subscribe.NewClient(hubSrv.URL, 10*time.Second)
	opts := processOpts{
		hubURL:     hubSrv.URL,
		catalogURL: catalogSrv.URL,
		secret:     secret,
		client:     client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = processDispatch(ctx, dispatch, "evt-002", opts)
	if err != nil {
		t.Fatalf("processDispatch should not return error on failed run (already acked), got: %v", err)
	}

	actualStatus, _ := resultStatus.Load().(string)
	if actualStatus != "failed" {
		t.Errorf("expected result status 'failed', got %q", actualStatus)
	}
	if !ackCalled.Load() {
		t.Error("ack should be called even after failed run")
	}
}

func TestDispatch_AcknowledgesMessage(t *testing.T) {
	secret := "test-runner-secret-key-32bytes!!"
	key := []byte(secret)

	// Helper to run a dispatch lifecycle and return whether ack was called.
	testAck := func(t *testing.T, status string) bool {
		t.Helper()

		g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, key)
		if err != nil {
			t.Fatal(err)
		}
		rawGrant, _ := json.Marshal(g)

		catalogMux := http.NewServeMux()
		catalogMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(transport.Artifact{
				Ref:     transport.AgentRef{Name: "agent", Version: "1.0"},
				Form:    transport.FormDefinition,
				Content: []byte("work"),
			})
		})
		catalogSrv := httptest.NewServer(catalogMux)
		defer catalogSrv.Close()

		var acked bool
		var mu sync.Mutex
		hubMux := http.NewServeMux()
		hubMux.HandleFunc("/api/v1/runners/sessions/sess-ack/result", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		hubMux.HandleFunc("/api/v1/jobs/messages/evt-ack/ack", func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			acked = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		})
		hubSrv := httptest.NewServer(hubMux)
		defer hubSrv.Close()

		origRunAgent := runAgent
		if status == "done" {
			runAgent = func(ctx context.Context, artifact *transport.Artifact) core.Result {
				return core.Result{Output: "ok", ExitCode: 0}
			}
		} else {
			runAgent = func(ctx context.Context, artifact *transport.Artifact) core.Result {
				return core.Result{Error: "fail", ExitCode: 1}
			}
		}
		defer func() { runAgent = origRunAgent }()

		dispatch := transport.Dispatch{
			Agent:   transport.AgentRef{Name: "agent", Version: "1.0"},
			Grant:   rawGrant,
			Session: "sess-ack",
			Runner:  "test-runner",
		}

		client := subscribe.NewClient(hubSrv.URL, 10*time.Second)
		opts := processOpts{
			hubURL:     hubSrv.URL,
			catalogURL: catalogSrv.URL,
			secret:     secret,
			client:     client,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_ = processDispatch(ctx, dispatch, "evt-ack", opts)

		mu.Lock()
		defer mu.Unlock()
		return acked
	}

	t.Run("ack after success", func(t *testing.T) {
		if !testAck(t, "done") {
			t.Error("ack was not called after successful dispatch")
		}
	})
	t.Run("ack after failure", func(t *testing.T) {
		if !testAck(t, "failed") {
			t.Error("ack was not called after failed dispatch")
		}
	})
}

func TestDispatch_CrashRecovery(t *testing.T) {
	// Crash recovery: a dispatch published to the broker before the engine
	// starts is delivered when the engine subscribes, simulating a scenario
	// where the engine crashed mid-run and the not-yet-acked dispatch is
	// replayed by the broker on restart.

	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-runner", accountID: "test-account-1"}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	// Publish a dispatch to the broker before the engine starts (simulating
	// a retained message that was not yet acknowledged).
	topic := bus.FormatTopic(bus.TopicRunnerDispatch, "test-runner")
	dispatch := transport.Dispatch{
		Agent:   transport.AgentRef{Name: "recovery-agent", Version: "1.0"},
		Grant:   json.RawMessage(`{}`),
		Session: "sess-recovery",
		Runner:  "test-runner",
	}
	if err := broker.Publish(context.Background(), topic, bus.Message{
		Payload: mustMarshal(t, dispatch),
	}); err != nil {
		t.Fatalf("publish pre-existing dispatch: %v", err)
	}

	// Now start the engine — it should receive the pending dispatch.
	eng := NewEngine("test-runner", "test-secret", server.URL, "http://catalog.local")
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

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

	select {
	case received := <-dispatchReceived:
		if received.Session != "sess-recovery" {
			t.Errorf("expected session 'sess-recovery', got %q", received.Session)
		}
		if received.Agent.Name != "recovery-agent" {
			t.Errorf("expected agent 'recovery-agent', got %q", received.Agent.Name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not receive the pre-existing dispatch within 5 seconds")
	}
}
