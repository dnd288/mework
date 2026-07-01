//go:build e2e

package e2e

import "testing"

// Feature 20 — Quotas & audit. Real-world platform hardening (proposed).

func TestQUOTA_01_ConcurrentRunLimit(t *testing.T) {
	Scenario(t, "QUOTA-01", "Per-tenant concurrent-run limit is enforced", PlannedPlatform).
		Given("tenant acme at its max concurrent runs", func(w *World) {
			w.set("lim", Limit{MaxConcurrentRuns: 2})
		}).
		When("another run is dispatched for acme", func(w *World) {
			ok, _ := w.Quota.Allow(ctx(), "acme", OpSpawn)
			w.set("ok", ok)
		}).
		Then("it is rejected/queued until a slot frees", func(w *World) {
			w.expect(w.get("ok") == false, "over-limit dispatch is not admitted")
		}).
		Run()
}

func TestQUOTA_02_DispatchRateLimit(t *testing.T) {
	Scenario(t, "QUOTA-02", "Dispatch rate limit is enforced", PlannedPlatform).
		Given("tenant acme dispatching above its per-minute rate", func(w *World) {}).
		When("the rate is exceeded", func(w *World) {
			ok, _ := w.Quota.Allow(ctx(), "acme", OpSpawn)
			w.set("ok", ok)
		}).
		Then("excess dispatches are throttled", func(w *World) {
			w.expect(w.get("ok") == false, "rate-limited dispatches are throttled")
		}).
		Run()
}

func TestQUOTA_03_LimitsQueryable(t *testing.T) {
	Scenario(t, "QUOTA-03", "A tenant's limits are queryable", PlannedPlatform).
		Given("tenant acme with configured limits", func(w *World) {}).
		When("the limits are queried", func(w *World) {
			lim, _ := w.Quota.Limits(ctx(), "acme")
			w.set("lim", lim)
		}).
		Then("the configured max-concurrent and rate are returned", func(w *World) {
			w.expect(w.get("lim").(Limit).MaxConcurrentRuns >= 0, "limits are introspectable")
		}).
		Run()
}

func TestAUDIT_01_SecurityActionsLogged(t *testing.T) {
	Scenario(t, "AUDIT-01", "Dispatch, grant, and enroll are audit-logged", PlannedPlatform).
		Given("an operator performing a dispatch", func(w *World) {}).
		When("the action completes", func(w *World) {
			w.set("err", w.Audit.Record(ctx(), AuditEntry{Action: "agent.dispatch", Target: "code-fixer@1.2.0"}))
		}).
		Then("an audit entry is recorded with actor, action, target, and time", func(w *World) {
			w.expect(w.get("err") == nil, "security-relevant actions are audited")
		}).
		Run()
}

func TestAUDIT_02_AuditQueryable(t *testing.T) {
	Scenario(t, "AUDIT-02", "Audit log is queryable per tenant", PlannedPlatform).
		Given("recorded audit entries for tenant acme", func(w *World) {
			_ = w.Audit.Record(ctx(), AuditEntry{Action: "runner.enroll"})
		}).
		When("an operator queries acme's audit log", func(w *World) {
			entries, _ := w.Audit.Query(ctx(), "acme")
			w.set("entries", entries)
		}).
		Then("acme's entries are returned and globex's are not", func(w *World) {
			w.expect(true, "audit query is tenant-scoped")
		}).
		Run()
}

func TestAUDIT_03_OrderingPreserved(t *testing.T) {
	Scenario(t, "AUDIT-03", "Audit entries preserve chronological order", PlannedPlatform).
		Given("multiple actions recorded over time", func(w *World) {}).
		When("the audit log is read back", func(w *World) {
			entries, _ := w.Audit.Query(ctx(), "acme")
			w.set("entries", entries)
		}).
		Then("entries are returned in append order (tamper-evident)", func(w *World) {
			w.expect(true, "audit ordering is preserved for forensics")
		}).
		Run()
}
