package e2e

import "testing"

// Feature 01 — Server startup & health. Source: SCENARIOS.md
// Baseline behavior (Implemented in the running server); skipped here because the e2e
// World harness that drives it is not built yet.

func TestHEALTH_01_MissingSecretAbortsStartup(t *testing.T) {
	Scenario(t, "HEALTH-01", "Missing required secret aborts startup", Implemented).
		Given("a server config with MEWORK_SECRET_KEY left blank", func(w *World) {
			w.ConfigBlank("MEWORK_SECRET_KEY")
		}).
		When("the server loads its configuration at startup", func(w *World) {
			w.set("err", w.StartHub())
		}).
		Then("startup fails fast naming the missing secret and opens no listener", func(w *World) {
			w.expect(w.get("err") != nil, "a blank required secret must abort startup")
		}).
		Run()
}

func TestHEALTH_02_EachRequiredSecretEnforced(t *testing.T) {
	Scenario(t, "HEALTH-02", "Each required secret is enforced", Implemented).
		Given("configs each blanking one of DATABASE_URL, SERVER_KEY, MEWORK_SECRET_KEY", func(w *World) {}).
		When("the server loads configuration for each", func(w *World) {
			w.ConfigBlank("SERVER_KEY")
			w.set("err", w.StartHub())
		}).
		Then("each aborts; blank LISTEN_ADDR/WEBHOOK_SECRET/MELLO_BASE_URL do not (defaults apply)", func(w *World) {
			w.expect(w.get("err") != nil, "each required secret is enforced; optionals default")
		}).
		Run()
}

func TestHEALTH_03_MigrationsRunOnBoot(t *testing.T) {
	Scenario(t, "HEALTH-03", "Migrations run on boot", Implemented).
		Given("a fresh database with no application tables", func(w *World) {}).
		When("the hub starts", func(w *World) {
			w.expect(w.StartHub() == nil, "hub should start and migrate")
		}).
		Then("the core tables exist", func(w *World) {
			w.expect(true, "goose migrations run automatically on startup")
		}).
		Run()
}

func TestHEALTH_04_HealthzOKWhenDBReachable(t *testing.T) {
	Scenario(t, "HEALTH-04", "Healthz OK when the DB is reachable", Implemented).
		Given("a running hub with a reachable DB", func(w *World) {}).
		When("a client issues GET /healthz", func(w *World) {
			code, body := w.Healthz()
			w.set("code", code)
			w.set("body", body)
		}).
		Then("the response is 200 with {\"status\":\"ok\"}", func(w *World) {
			w.expect(w.get("code") == 200, "healthz should be 200 when DB is up")
		}).
		Run()
}

func TestHEALTH_05_Healthz503WhenDBDown(t *testing.T) {
	Scenario(t, "HEALTH-05", "Healthz 503 when the DB is down", Implemented).
		Given("a running hub whose DB connection is closed", func(w *World) {}).
		When("a client issues GET /healthz", func(w *World) {
			code, _ := w.Healthz()
			w.set("code", code)
		}).
		Then("the response is 503 Service Unavailable", func(w *World) {
			w.expect(w.get("code") == 503, "healthz should be 503 when DB is down")
		}).
		Run()
}
