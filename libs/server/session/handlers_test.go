package session

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mework/libs/server/auth"
	"mework/libs/server/bus/memory"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// fakeDispatcher records the arguments passed to DispatchSessionToRunner.
type fakeDispatcher struct {
	calls   int
	agent   string
	runner  string
	session string
	owner   string
	tenant  string
	grant   *grant.Grant
	err     error
}

func (f *fakeDispatcher) DispatchSessionToRunner(ctx context.Context, agentName, runnerID, sessionID, owner, tenant string, g *grant.Grant) error {
	f.calls++
	f.agent = agentName
	f.runner = runnerID
	f.session = sessionID
	f.owner = owner
	f.tenant = tenant
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
	h := NewHandlers(mgr, disp)

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

func TestListSessions_TenantScoped(t *testing.T) {
	mgr := NewManager(memory.New(), DefaultConfig())
	defer mgr.Stop()
	disp := &fakeDispatcher{}
	h := NewHandlers(mgr, disp)

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
	h := NewHandlers(mgr, &fakeDispatcher{})

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
	h := NewHandlers(mgr, &fakeDispatcher{})

	body, _ := json.Marshal(map[string]string{"status": "done", "summary": "ok"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners/sessions/sess-1/result", bytes.NewReader(body))
	req.SetPathValue("id", "sess-1")
	rec := httptest.NewRecorder()

	h.ResultSession(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}
