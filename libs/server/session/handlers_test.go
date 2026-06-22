package session

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mework/libs/server/auth"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// fakeDispatcher records the arguments passed to DispatchSessionToRunner.
type fakeDispatcher struct {
	calls     int
	agent     string
	runner    string
	session   string
	owner     string
	tenant    string
	workspace string
	grant     *grant.Grant
	err       error
}

func (f *fakeDispatcher) DispatchSessionToRunner(ctx context.Context, agentName, runnerID, sessionID, owner, tenant, workspace string, g *grant.Grant) error {
	f.calls++
	f.agent = agentName
	f.runner = runnerID
	f.session = sessionID
	f.owner = owner
	f.tenant = tenant
	f.workspace = workspace
	f.grant = g
	return f.err
}

func withAuth(r *http.Request, account, tenant string) *http.Request {
	ctx := context.WithValue(r.Context(), auth.AccountIDKey, account)
	ctx = context.WithValue(ctx, auth.TenantIDKey, tenant)
	return r.WithContext(ctx)
}

func TestCreateSession_DispatchesAndUsesAuthContext(t *testing.T) {
	mgr := NewManager(memory.New(), DefaultConfig())
	defer mgr.Stop()
	disp := &fakeDispatcher{}
	h := NewHandlers(mgr, disp, memory.New())

	body, _ := json.Marshal(map[string]string{"agent_name": "code-fixer", "runner": "rnr-1"})
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(body)), "acct-7", "tenant-9")
	rec := httptest.NewRecorder()

	h.CreateSession(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var info core.SessionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode SessionInfo: %v", err)
	}
	if info.ID == "" {
		t.Fatal("expected non-empty session id")
	}
	if info.Owner != core.AccountID("acct-7") {
		t.Errorf("owner = %q, want acct-7 (from auth context)", info.Owner)
	}
	if info.Tenant != core.TenantID("tenant-9") {
		t.Errorf("tenant = %q, want tenant-9 (from auth context)", info.Tenant)
	}

	if disp.calls != 1 {
		t.Fatalf("dispatcher called %d times, want 1", disp.calls)
	}
	if disp.session != string(info.ID) {
		t.Errorf("dispatched session = %q, want %q", disp.session, info.ID)
	}
	if disp.owner != "acct-7" || disp.tenant != "tenant-9" {
		t.Errorf("dispatched owner/tenant = %q/%q, want acct-7/tenant-9", disp.owner, disp.tenant)
	}
	if disp.runner != "rnr-1" || disp.agent != "code-fixer" {
		t.Errorf("dispatched runner/agent = %q/%q, want rnr-1/code-fixer", disp.runner, disp.agent)
	}
	if disp.grant == nil || !disp.grant.Permits(grant.OpPullAgent) || !disp.grant.Permits(grant.OpSpawn) {
		t.Errorf("dispatched grant must permit pull+spawn, got %+v", disp.grant)
	}
}

func TestCreateSession_ForwardsWorkspace(t *testing.T) {
	mgr := NewManager(memory.New(), DefaultConfig())
	defer mgr.Stop()
	disp := &fakeDispatcher{}
	h := NewHandlers(mgr, disp, memory.New())

	body, _ := json.Marshal(map[string]string{
		"agent_name": "local-claude", "runner": "rnr-1", "workspace": "/abs/ws/proj",
	})
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewReader(body)), "acct-7", "tenant-9")
	rec := httptest.NewRecorder()

	h.CreateSession(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if disp.calls != 1 {
		t.Fatalf("dispatcher called %d times, want 1", disp.calls)
	}
	if disp.workspace != "/abs/ws/proj" {
		t.Errorf("dispatched workspace = %q, want /abs/ws/proj", disp.workspace)
	}
}

func TestListSessions_TenantScoped(t *testing.T) {
	mgr := NewManager(memory.New(), DefaultConfig())
	defer mgr.Stop()
	disp := &fakeDispatcher{}
	h := NewHandlers(mgr, disp, memory.New())

	ctx := context.Background()
	if _, err := mgr.Create(ctx, "a", "", "r1", "acct-1", "tenant-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create(ctx, "a", "", "r2", "acct-2", "tenant-B"); err != nil {
		t.Fatal(err)
	}

	req := withAuth(httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil), "acct-1", "tenant-A")
	rec := httptest.NewRecorder()
	h.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var list []core.SessionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0].Tenant != core.TenantID("tenant-A") {
		t.Fatalf("tenant scoping failed: %+v", list)
	}
}

func TestGetAndCloseSession(t *testing.T) {
	mgr := NewManager(memory.New(), DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, &fakeDispatcher{}, memory.New())

	info, err := mgr.Create(context.Background(), "a", "", "r1", "acct-1", "tenant-A")
	if err != nil {
		t.Fatal(err)
	}

	// GET
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+string(info.ID), nil)
	req.SetPathValue("id", string(info.ID))
	rec := httptest.NewRecorder()
	h.GetSession(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}

	// DELETE
	dreq := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+string(info.ID), nil)
	dreq.SetPathValue("id", string(info.ID))
	drec := httptest.NewRecorder()
	h.CloseSession(drec, dreq)
	if drec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", drec.Code)
	}

	got, _ := mgr.Get(context.Background(), info.ID)
	if got.Status != core.SessionClosed {
		t.Errorf("status after close = %q, want closed", got.Status)
	}
}

func TestResultSession_204(t *testing.T) {
	mgr := NewManager(memory.New(), DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, &fakeDispatcher{}, memory.New())

	body, _ := json.Marshal(map[string]string{"status": "done", "summary": "ok"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners/sessions/sess-1/result", bytes.NewReader(body))
	req.SetPathValue("id", "sess-1")
	rec := httptest.NewRecorder()

	h.ResultSession(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}

// newOwnedSession creates a session owned by ownerAcct and returns its id.
func newOwnedSession(t *testing.T, mgr *Manager, ownerAcct string) core.SessionID {
	t.Helper()
	ctx := withAccount(context.Background(), ownerAcct, "tenant-1")
	info, err := mgr.Create(ctx, "agent", "v1", "runner-1", core.AccountID(ownerAcct), core.TenantID("tenant-1"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return info.ID
}

func withAccount(ctx context.Context, acct, tenant string) context.Context {
	ctx = context.WithValue(ctx, auth.AccountIDKey, acct)
	ctx = context.WithValue(ctx, auth.TenantIDKey, tenant)
	return ctx
}

// --- SendMessage --------------------------------------------------------------

// TestSendMessage_OwnerPublishesToInput verifies that the owning caller's turn is
// published to session.<id>.input and the call returns 202.
// Delta-spec scenario: "Submit a chat turn".
func TestSendMessage_OwnerPublishesToInput(t *testing.T) {
	broker := memory.New()
	mgr := NewManager(broker, DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, nil, broker)

	id := newOwnedSession(t, mgr, "owner-1")

	// Subscribe to the input topic to assert the turn lands there.
	inputSub, err := broker.Subscribe(context.Background(), bus.Identity("runner"),
		bus.Filter(bus.FormatTopic(bus.TopicSessionInput, string(id))), "")
	if err != nil {
		t.Fatalf("subscribe input: %v", err)
	}
	defer inputSub.Close()

	body, _ := json.Marshal(ChatMessage{Role: RoleUser, Content: "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+string(id)+"/messages", bytes.NewReader(body))
	req = req.WithContext(withAccount(req.Context(), "owner-1", "tenant-1"))
	req.SetPathValue("id", string(id))
	rec := httptest.NewRecorder()

	h.SendMessage(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}

	select {
	case ev := <-inputSub.Events():
		var msg ChatMessage
		if err := json.Unmarshal(ev.Message.Payload, &msg); err != nil {
			t.Fatalf("unmarshal published turn: %v", err)
		}
		if msg.Content != "hello" || msg.Role != RoleUser {
			t.Fatalf("published turn = %+v, want hello/user", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("turn was not published to the input topic")
	}
}

// TestSendMessage_NonOwnerDenied verifies a non-owner cannot submit a turn and
// nothing is published. Delta-spec scenario: "Non-owner cannot submit or stream".
func TestSendMessage_NonOwnerDenied(t *testing.T) {
	broker := memory.New()
	mgr := NewManager(broker, DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, nil, broker)

	id := newOwnedSession(t, mgr, "owner-1")

	inputSub, err := broker.Subscribe(context.Background(), bus.Identity("runner"),
		bus.Filter(bus.FormatTopic(bus.TopicSessionInput, string(id))), "")
	if err != nil {
		t.Fatalf("subscribe input: %v", err)
	}
	defer inputSub.Close()

	body, _ := json.Marshal(ChatMessage{Role: RoleUser, Content: "intruder"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+string(id)+"/messages", bytes.NewReader(body))
	req = req.WithContext(withAccount(req.Context(), "attacker", "tenant-1"))
	req.SetPathValue("id", string(id))
	rec := httptest.NewRecorder()

	h.SendMessage(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}

	select {
	case ev := <-inputSub.Events():
		t.Fatalf("non-owner turn leaked to input: %q", ev.Message.Payload)
	case <-time.After(100 * time.Millisecond):
	}
}

// --- StreamSession ------------------------------------------------------------

// TestStreamSession_OwnerReceivesControlEvent verifies an event published to
// session.<id>.control is relayed as an SSE data frame to the owner.
// Delta-spec scenarios: "Stream session events", "Outgoing events flow on the control topic".
func TestStreamSession_OwnerReceivesControlEvent(t *testing.T) {
	broker := memory.New()
	mgr := NewManager(broker, DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, nil, broker)

	id := newOwnedSession(t, mgr, "owner-1")

	ctx, cancel := context.WithCancel(withAccount(context.Background(), "owner-1", "tenant-1"))
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+string(id)+"/stream", nil).WithContext(ctx)
	req.SetPathValue("id", string(id))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.StreamSession(rec, req)
		close(done)
	}()

	// Give the handler time to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)

	ev := ChatEvent{Kind: EventToken, Content: "hi"}
	payload, _ := json.Marshal(ev)
	if err := broker.Publish(context.Background(),
		bus.FormatTopic(bus.TopicSessionControl, string(id)), bus.Message{Payload: payload}); err != nil {
		t.Fatalf("publish control: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(rec.Body.String(), `"content":"hi"`) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("control event not relayed as SSE; got body=%q", rec.Body.String())
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	cancel()
	<-done
}

// TestStreamSession_NonOwnerDenied verifies a non-owner is denied before subscribing.
func TestStreamSession_NonOwnerDenied(t *testing.T) {
	broker := memory.New()
	mgr := NewManager(broker, DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, nil, broker)

	id := newOwnedSession(t, mgr, "owner-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+string(id)+"/stream", nil)
	req = req.WithContext(withAccount(req.Context(), "attacker", "tenant-1"))
	req.SetPathValue("id", string(id))
	rec := httptest.NewRecorder()

	h.StreamSession(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- ReceiveEvents (runner ingress) ------------------------------------------

// TestReceiveEvents_RepublishesToControl verifies the runner events-ingress
// republishes a ChatEvent to session.<id>.control.
// Delta-spec scenario: "Runner delivers events for relay".
func TestReceiveEvents_RepublishesToControl(t *testing.T) {
	broker := memory.New()
	mgr := NewManager(broker, DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, nil, broker)

	id := newOwnedSession(t, mgr, "owner-1")

	controlSub, err := broker.Subscribe(context.Background(), bus.Identity("hub"),
		bus.Filter(bus.FormatTopic(bus.TopicSessionControl, string(id))), "")
	if err != nil {
		t.Fatalf("subscribe control: %v", err)
	}
	defer controlSub.Close()

	body, _ := json.Marshal(ChatEvent{Kind: EventMessage, Content: "reply"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners/sessions/"+string(id)+"/events", bytes.NewReader(body))
	req.SetPathValue("id", string(id))
	rec := httptest.NewRecorder()

	h.ReceiveEvents(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}

	select {
	case ev := <-controlSub.Events():
		var got ChatEvent
		if err := json.Unmarshal(ev.Message.Payload, &got); err != nil {
			t.Fatalf("unmarshal republished event: %v", err)
		}
		if got.Content != "reply" || got.Kind != EventMessage {
			t.Fatalf("republished event = %+v, want reply/message", got)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not republished to the control topic")
	}
}

// TestReceiveEvents_MissingID rejects requests without a session id.
func TestReceiveEvents_MissingID(t *testing.T) {
	broker := memory.New()
	mgr := NewManager(broker, DefaultConfig())
	defer mgr.Stop()
	h := NewHandlers(mgr, nil, broker)

	body, _ := json.Marshal(ChatEvent{Kind: EventDone})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners/sessions//events", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ReceiveEvents(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
