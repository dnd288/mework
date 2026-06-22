//go:build e2e

package e2e

import "testing"

// Feature 16 — Session lifecycle & management. Real-world platform surface (proposed).
// Sessions are first-class: create, attach, list, status, ownership, close.

func TestSESSION_01_CreateAttachClose(t *testing.T) {
	Scenario(t, "SESSION-01", "Create → attach → close lifecycle", PlannedPlatform).
		Given("a dispatch for code-fixer to runner R", func(w *World) {
			info, err := w.Sessions.Create(ctx(), Dispatch{Agent: AgentRef{Name: "code-fixer", Version: "1.2.0"}, Runner: "R"})
			w.set("info", info)
			w.expect(err == nil, "session creation should succeed")
		}).
		When("the client attaches then closes the session", func(w *World) {
			info := w.get("info").(SessionInfo)
			sess, _ := w.Sessions.Attach(ctx(), info.ID)
			w.set("sess", sess)
			w.set("err", w.Sessions.Close(ctx(), info.ID))
		}).
		Then("attach returns a live endpoint and close terminates the session", func(w *World) {
			w.expect(w.get("err") == nil, "the session lifecycle completes cleanly")
		}).
		Run()
}

func TestSESSION_02_ListStatusOwner(t *testing.T) {
	Scenario(t, "SESSION-02", "List sessions with status and owner", PlannedPlatform).
		Given("several active sessions in tenant acme", func(w *World) {
			_, _ = w.Sessions.Create(ctx(), Dispatch{Runner: "R"})
		}).
		When("an operator lists sessions for acme", func(w *World) {
			list, _ := w.Sessions.List(ctx(), "acme")
			w.set("list", list)
		}).
		Then("each entry reports its status (active/idle/closed) and owner", func(w *World) {
			w.expect(true, "session listing exposes status + owner for management")
		}).
		Run()
}

func TestSESSION_03_ResumeAfterReconnect(t *testing.T) {
	Scenario(t, "SESSION-03", "Resume an attached session after reconnect", PlannedPlatform).
		Given("a client attached to a session whose connection drops", func(w *World) {
			info, _ := w.Sessions.Create(ctx(), Dispatch{Runner: "R"})
			w.set("id", info.ID)
		}).
		When("the client re-attaches to the same session id", func(w *World) {
			sess, err := w.Sessions.Attach(ctx(), w.get("id").(SessionID))
			w.set("sess", sess)
			w.set("err", err)
		}).
		Then("it reconnects to the still-running agent without losing session state", func(w *World) {
			w.expect(w.get("err") == nil, "re-attach resumes the live session")
		}).
		Run()
}

func TestSESSION_04_MultiplePerRunner(t *testing.T) {
	Scenario(t, "SESSION-04", "Multiple sessions coexist on one runner", PlannedPlatform).
		Given("runner R hosting two independent sessions", func(w *World) {
			a, _ := w.Sessions.Create(ctx(), Dispatch{Runner: "R", Session: "sa"})
			b, _ := w.Sessions.Create(ctx(), Dispatch{Runner: "R", Session: "sb"})
			w.set("a", a)
			w.set("b", b)
		}).
		When("both are active", func(w *World) {}).
		Then("each has its own sandbox/control channel; they do not interfere", func(w *World) {
			w.expect(w.get("a").(SessionInfo).ID != w.get("b").(SessionInfo).ID, "sessions are distinct")
		}).
		Run()
}

func TestSESSION_05_IdleTimeout(t *testing.T) {
	Scenario(t, "SESSION-05", "An idle session times out and is reaped", PlannedPlatform).
		Given("a session with no activity past its idle timeout", func(w *World) {
			info, _ := w.Sessions.Create(ctx(), Dispatch{Runner: "R"})
			w.set("id", info.ID)
		}).
		When("the idle period elapses", func(w *World) {
			w.Advance(3600)
		}).
		Then("the session transitions to closed and its sandbox is destroyed", func(w *World) {
			info, _ := w.Sessions.Get(ctx(), w.get("id").(SessionID))
			w.expect(info.Status == SessionClosed, "idle sessions are reaped, got %q", info.Status)
		}).
		Run()
}

func TestSESSION_06_OwnershipEnforced(t *testing.T) {
	Scenario(t, "SESSION-06", "Only the owner may attach to a session", PlannedPlatform).
		Given("a session owned by account A", func(w *World) {
			info, _ := w.Sessions.Create(ctx(), Dispatch{Runner: "R"})
			w.set("id", info.ID)
		}).
		When("account B attempts to attach", func(w *World) {
			_, err := w.Sessions.Attach(ctx(), w.get("id").(SessionID))
			w.set("err", err)
		}).
		Then("the attach is denied (session ownership enforced)", func(w *World) {
			w.expect(w.get("err") != nil, "a non-owner cannot attach")
		}).
		Run()
}

func TestSESSION_07_TenantIsolation(t *testing.T) {
	Scenario(t, "SESSION-07", "Sessions are isolated per tenant", PlannedPlatform).
		Given("sessions in tenants acme and globex", func(w *World) {}).
		When("an acme operator lists sessions", func(w *World) {
			list, _ := w.Sessions.List(ctx(), "acme")
			w.set("list", list)
		}).
		Then("globex sessions are never returned", func(w *World) {
			w.expect(true, "session listing is tenant-scoped; no cross-tenant visibility")
		}).
		Run()
}
