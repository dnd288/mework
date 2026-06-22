package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/libs/client/subscribe"
	"mework/libs/sandbox"
	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/grant"
	"mework/libs/shared/transport"
)

// TestSessionInput_ControlClosesAndCancels asserts that a "close" control
// message on the input topic closes the session (destroying the sandbox) and
// removes it from the registry, and that a "cancel" control message interrupts
// the in-flight turn without destroying the sandbox. Realises the lifecycle
// mapping in delta-spec requirement "Open-session dispatch drives the
// interactive session".
func TestSessionInput_ControlClosesAndCancels(t *testing.T) {
	deps, drv, _ := newSessionDeps(t, 0, false)
	caller := ownerCaller(t)
	ctx := context.Background()

	sess, err := OpenSession(ctx, "local-claude@1.0.0", caller, deps)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	eng := NewEngine("test-runner", "test-secret", "http://hub.local", "http://catalog.local")
	const sessionID = "sess-ctl"
	eng.registerSession(sessionID, sess)

	// A cancel control message interrupts the (idle) turn but keeps the sandbox.
	cancelMsg, _ := json.Marshal(inputMessage{Control: "cancel"})
	if done := eng.handleSessionInput(ctx, sessionID, sess, caller, bus.Event{Message: bus.Message{Payload: cancelMsg}}); done {
		t.Error("cancel control must not end the input loop")
	}
	if got := drv.destroys(); got != 0 {
		t.Errorf("cancel destroyed the sandbox %d time(s); it must stay alive", got)
	}
	if _, ok := eng.lookupSession(sessionID); !ok {
		t.Error("cancel must not remove the session from the registry")
	}

	// A close control message destroys the sandbox and ends the loop.
	closeMsg, _ := json.Marshal(inputMessage{Control: "close"})
	if done := eng.handleSessionInput(ctx, sessionID, sess, caller, bus.Event{Message: bus.Message{Payload: closeMsg}}); !done {
		t.Error("close control must end the input loop")
	}
	if got := drv.destroys(); got < 1 {
		t.Errorf("close must destroy the long-lived sandbox, destroys=%d", got)
	}
	if _, ok := eng.lookupSession(sessionID); ok {
		t.Error("close must remove the session from the registry")
	}
}

// TestProcessSessionDispatch_OpenAndTurns drives a full open-session dispatch:
// the engine opens one long-lived sandbox, acks the dispatch, then two chat
// turns published on the session's input topic run on the SAME sandbox, and each
// turn's token/message/done events are egressed to the server's events endpoint.
// Realises delta-spec scenarios "Open-session dispatch starts one long-lived
// sandbox", "Turns from the input topic route to the session", and "Per-turn
// events reach the server".
func TestProcessSessionDispatch_OpenAndTurns(t *testing.T) {
	const sessionID = "sess-int-1"
	const secret = "test-runner-secret-key-32bytes!!"

	// Shared in-memory broker drives the input-topic SSE subscription.
	inputBroker := memory.New()
	sseHandler := bus.NewSSEHandler(inputBroker)

	// Capture egressed events posted to the events-ingress endpoint.
	var evMu sync.Mutex
	var gotEventKinds []string
	var ackCount int

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-runner", accountID: "owner-acct"}.Middleware)
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)
	mux.Post("/api/v1/jobs/messages/{id}/ack", func(w http.ResponseWriter, r *http.Request) {
		evMu.Lock()
		ackCount++
		evMu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.Post("/api/v1/runners/sessions/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		var ev session.ChatEvent
		_ = json.NewDecoder(r.Body).Decode(&ev)
		evMu.Lock()
		gotEventKinds = append(gotEventKinds, string(ev.Kind))
		evMu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Fake catalog resolver returning a local-engine stub backend.
	defs := map[string]sandbox.SandboxBundleMetadata{
		"local-claude@1.0.0": {Name: "local-claude", Version: "1.0.0", Engine: "local", Backend: "claude"},
	}
	origResolver := sessionResolverFor
	sessionResolverFor = func(string) DefinitionResolver { return fakeResolver{defs: defs} }
	t.Cleanup(func() { sessionResolverFor = origResolver })

	// Fake driver: one long-lived sandbox, counts Start and records turns.
	drv := &liveFakeDriver{}

	// Route OpenSession's ManagerFor (inside the session deps) to our fake driver.
	origMgrFor := sessionRuntimeManagerFor
	sessionRuntimeManagerFor = func(string) *runtime.Manager { return runtime.NewManager(drv) }
	t.Cleanup(func() { sessionRuntimeManagerFor = origMgrFor })

	// The broker the session publishes its events to is the real httpBroker, so
	// they egress to the server's events endpoint.
	origBrokerFor := sessionBrokerFor
	sessionBrokerFor = func(hubURL, sec string) bus.Broker { return newHTTPBroker(srv.URL, sec) }
	t.Cleanup(func() { sessionBrokerFor = origBrokerFor })

	eng := NewEngine("test-runner", secret, srv.URL, srv.URL)
	eng.client = subscribe.NewClient(srv.URL, 0)

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpSpawn}, []byte(secret))
	if err != nil {
		t.Fatalf("new grant: %v", err)
	}
	rawGrant, _ := json.Marshal(g)

	d := transport.Dispatch{
		Agent:   transport.AgentRef{Name: "local-claude", Version: "1.0.0"},
		Grant:   rawGrant,
		Session: sessionID,
		Runner:  "test-runner",
		Owner:   "owner-acct",
		Tenant:  "tenant-a",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer eng.Stop()

	opts := processOpts{hubURL: srv.URL, catalogURL: srv.URL, secret: secret, client: eng.client}
	if err := processSessionDispatch(ctx, eng, d, "evt-open", opts); err != nil {
		t.Fatalf("processSessionDispatch: %v", err)
	}

	// The session must be registered exactly once.
	if _, ok := eng.lookupSession(sessionID); !ok {
		t.Fatal("session was not registered after open")
	}

	// Publish two turns on the input topic.
	inputTopic := bus.FormatTopic(bus.TopicSessionInput, sessionID)
	for _, content := range []string{"turn one", "turn two"} {
		payload, _ := json.Marshal(inputMessage{Message: session.ChatMessage{Role: session.RoleUser, Content: content}})
		if err := inputBroker.Publish(ctx, inputTopic, bus.Message{Payload: payload}); err != nil {
			t.Fatalf("publish input turn: %v", err)
		}
	}

	// Wait until both turns have been delivered to the one sandbox.
	deadline := time.After(5 * time.Second)
	for {
		if drv.sb != nil && len(drv.sb.gotTurns()) >= 2 {
			break
		}
		select {
		case <-deadline:
			n := 0
			if drv.sb != nil {
				n = len(drv.sb.gotTurns())
			}
			t.Fatalf("only %d turns delivered, want 2", n)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if got := drv.starts(); got != 1 {
		t.Errorf("sandbox Start called %d times, want exactly 1 (one long-lived sandbox)", got)
	}
	turns := drv.sb.gotTurns()
	if turns[0] != "turn one" || turns[1] != "turn two" {
		t.Errorf("turns delivered out of order: %v", turns)
	}

	// Each turn egresses token+message+done; wait for at least two done events.
	deadline = time.After(5 * time.Second)
	for {
		evMu.Lock()
		dones := 0
		for _, k := range gotEventKinds {
			if k == string(session.EventDone) {
				dones++
			}
		}
		evMu.Unlock()
		if dones >= 2 {
			break
		}
		select {
		case <-deadline:
			evMu.Lock()
			kinds := append([]string(nil), gotEventKinds...)
			evMu.Unlock()
			t.Fatalf("did not observe 2 done events egressed to the server, got kinds: %v", kinds)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
