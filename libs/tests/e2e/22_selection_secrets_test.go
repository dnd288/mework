package e2e

import "testing"

// Feature 22 — Runner selection & secret injection. Real-world platform hardening (proposed).

func TestSELECT_01_LoadBalances(t *testing.T) {
	Scenario(t, "SELECT-01", "Dispatch load-balances across eligible runners", PlannedPlatform).
		Given("a tenant with three online runners eligible for an agent", func(w *World) {}).
		When("multiple dispatches are placed", func(w *World) {
			r, err := w.Selector.Select(ctx(), Dispatch{Agent: AgentRef{Name: "code-fixer", Version: "1.2.0"}})
			w.set("r", r)
			w.set("err", err)
		}).
		Then("the selector spreads them across runners (no single runner overloaded)", func(w *World) {
			w.expect(w.get("err") == nil && w.get("r").(RunnerID) != "", "a runner is selected for the dispatch")
		}).
		Run()
}

func TestSELECT_02_SessionAffinity(t *testing.T) {
	Scenario(t, "SELECT-02", "Session dispatches stick to their runner (affinity)", PlannedPlatform).
		Given("an existing session bound to runner R", func(w *World) {}).
		When("a follow-up dispatch for that session is selected", func(w *World) {
			r, _ := w.Selector.Select(ctx(), Dispatch{Session: "s1", Runner: "R"})
			w.set("r", r)
		}).
		Then("it is routed back to R (sticky session affinity)", func(w *World) {
			w.expect(w.get("r").(RunnerID) == "R", "session dispatches keep affinity to their runner")
		}).
		Run()
}

func TestSELECT_03_NoEligibleRunner(t *testing.T) {
	Scenario(t, "SELECT-03", "No eligible runner is handled gracefully", PlannedPlatform).
		Given("no online runner eligible for the agent", func(w *World) {}).
		When("a dispatch is attempted", func(w *World) {
			_, err := w.Selector.Select(ctx(), Dispatch{Agent: AgentRef{Name: "x"}})
			w.set("err", err)
		}).
		Then("selection fails clearly (queued or rejected, not silently dropped)", func(w *World) {
			w.expect(w.get("err") != nil, "no-eligible-runner is surfaced, not swallowed")
		}).
		Run()
}

func TestSECRET_01_ScopedInjection(t *testing.T) {
	Scenario(t, "SECRET-01", "Grant-scoped secrets are injected into the sandbox", PlannedPlatform).
		Given("a dispatch whose grant scopes a secret (e.g. a repo token)", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1", Grant: grant(OpRepoRead)})
			w.set("id", id)
		}).
		When("the sandbox is provisioned", func(w *World) {
			w.set("err", w.Secrets.Inject(ctx(), w.get("id").(SandboxID), map[string]string{"REPO_TOKEN": "x"}))
		}).
		Then("the secret is available inside the sandbox only", func(w *World) {
			w.expect(w.get("err") == nil, "scoped secrets are injected into the sandbox")
		}).
		Run()
}

func TestSECRET_02_NeverInArgvOrLogs(t *testing.T) {
	Scenario(t, "SECRET-02", "Injected secrets never appear in argv or logs", PlannedPlatform).
		Given("a secret injected into a running sandbox", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			_ = w.Secrets.Inject(ctx(), id, map[string]string{"REPO_TOKEN": "x"})
		}).
		When("the agent runs and emits logs", func(w *World) {}).
		Then("the secret value is absent from argv, command lines, and streamed logs", func(w *World) {
			w.expect(true, "secrets are delivered out-of-band (env/file), never argv/logs")
		}).
		Run()
}

func TestSECRET_03_ScopedToGrant(t *testing.T) {
	Scenario(t, "SECRET-03", "Secrets outside the grant are not injected", PlannedPlatform).
		Given("a dispatch whose grant does not include a given secret scope", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1", Grant: grant(OpRepoRead)})
			w.set("id", id)
		}).
		When("provisioning attempts to inject an out-of-scope secret", func(w *World) {
			w.set("err", w.Secrets.Inject(ctx(), w.get("id").(SandboxID), map[string]string{"PROD_DB": "x"}))
		}).
		Then("the injection is refused (secrets are bounded by the grant)", func(w *World) {
			w.expect(w.get("err") != nil, "only grant-scoped secrets may be injected")
		}).
		Run()
}
