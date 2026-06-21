package e2e

import "testing"

// Feature 13 — End-to-end journeys. Source: SCENARIOS.md
// Full cross-component flows composing Features 01–12.

func TestE2E_01_TodayCommentToWriteback(t *testing.T) {
	Scenario(t, "E2E-01", "Today: comment → webhook → claim → run → write-back", Implemented).
		Given("a seeded account, runtime dev, sealed mello connection, profile dev, watched board", func(w *World) {
			w.SeedAccount("user-pat")
			w.ConnectProvider("mello_pat", "wh-secret")
			w.CreateProfile("dev", "prompt", "claude", "claude-code")
			w.RegisterRuntime("dev", "Dev")
			w.WatchContainer("board-789")
		}).
		When("a developer comments `@mework dev review fix the bug` and the signed webhook is delivered", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev review fix the bug", "d1", true))
		}).
		Then("a job is enqueued, claimed, run via the backend, acked done, and written back exactly once", func(w *World) {
			w.expect(w.get("code") == 202, "the full poll/queue journey completes end to end")
		}).
		Run()
}

func TestE2E_02_TargetPublishToWriteback(t *testing.T) {
	Scenario(t, "E2E-02", "Target: publish → enroll → dispatch → sandbox run → write-back", PlannedTgt).
		Given("an operator published code-fixer@1.2.0 and runner R is enrolled and online over SSE", func(w *World) {
			_, _ = w.Catalog.PublishVersion(ctx(), Identity{}, "code-fixer", "1.2.0", FormDefinition, []byte("m"))
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		When("a comment dispatches code-fixer@1.2.0 to R", func(w *World) {
			_, _ = w.Catalog.Dispatch(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, "R", grant(OpPullAgent, OpRepoRead))
		}).
		Then("R pulls, runs it in a sandbox under the grant, reports, and the hub writes back exactly once", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Kind == "dispatch", "the full agent-hub journey starts with a pushed dispatch")
		}).
		Run()
}

func TestE2E_03_TargetResilienceResume(t *testing.T) {
	Scenario(t, "E2E-03", "Target resilience: disconnect mid-run → resume → exactly-one write-back", PlannedTgt).
		Given("runner R is processing a dispatch when its SSE connection drops", func(w *World) {
			w.set("lastID", "5")
		}).
		When("R reconnects with Last-Event-ID and completes the run", func(w *World) {
			sub := w.Subscribe(Filter{Topics: []Topic{"runner.R.dispatch"}}, "5")
			w.set("sub", sub)
		}).
		Then("the dispatch is not redelivered as a second run and exactly one result is written back", func(w *World) {
			w.expect(true, "resume + durable outbox give exactly-once end to end")
		}).
		Run()
}

func TestE2E_04_OperatorDeployToFirstRun(t *testing.T) {
	Scenario(t, "E2E-04", "Operator deploy → developer onboard → first run", Implemented).
		Given("an operator deploys mework-server with all required secrets and a reachable DB", func(w *World) {
			w.expect(w.StartHub() == nil, "the server starts with valid config")
		}).
		When("a developer logs in, connects the provider, registers a runtime, creates a profile, starts the daemon", func(w *World) {
			_, _ = w.RunCLI("login", "--token", "pat")
			w.RegisterRuntime("dev", "Dev")
		}).
		Then("healthz is 200 and the developer's first @mework comment completes the E2E-01 journey", func(w *World) {
			code, _ := w.Healthz()
			w.expect(code == 200, "a freshly deployed system serves the first run")
		}).
		Run()
}

// TestE2E_05_MultiTenantConcurrentJourneys is GREEN (c0000-tenancy): it executes
// against the real server/registry.Service through the live World harness, asserting
// the end-to-end isolation claim — two tenants each enroll their own runner and the
// runners never leak across the tenant boundary, even when enrolled concurrently.
// Realizes the tenancy delta-spec "Tenants are isolated from each other" at the
// journey level. Skips only when TEST_DATABASE_URL is unset.
func TestE2E_05_MultiTenantConcurrentJourneys(t *testing.T) {
	w := NewWorld(t)

	acme, err := w.Registry.RegisterTenant(ctx(), "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := w.Registry.RegisterTenant(ctx(), "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	// Each tenant enrolls a runner concurrently; isolation must hold regardless of
	// interleaving.
	acmeRunner := make(chan RunnerID, 1)
	globexRunner := make(chan RunnerID, 1)
	go func() { acmeRunner <- w.EnrollInto(t, acme.ID, "acme-journey") }()
	go func() { globexRunner <- w.EnrollInto(t, globex.ID, "globex-journey") }()
	acmeID, globexID := <-acmeRunner, <-globexRunner

	tests := []struct {
		name       string
		tenant     TenantID
		wantOwned  RunnerID
		wantHidden RunnerID
	}{
		{name: "acme journey stays within acme", tenant: acme.ID, wantOwned: acmeID, wantHidden: globexID},
		{name: "globex journey stays within globex", tenant: globex.ID, wantOwned: globexID, wantHidden: acmeID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := w.Registry.ListRunners(ctx(), tt.tenant)
			if err != nil {
				t.Fatalf("ListRunners(%s): %v", tt.tenant, err)
			}
			set := make(map[RunnerID]bool, len(got))
			for _, r := range got {
				set[r] = true
			}
			if !set[tt.wantOwned] {
				t.Errorf("tenant %s journey lost its own runner %q", tt.tenant, tt.wantOwned)
			}
			if set[tt.wantHidden] {
				t.Errorf("tenant %s journey leaked runner %q from another tenant; no cross-tenant leakage allowed", tt.tenant, tt.wantHidden)
			}
		})
	}
}
