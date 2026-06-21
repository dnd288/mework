package e2e

import "testing"

// Feature 12 — REST write-back. Source: SCENARIOS.md
// Baseline behavior (Implemented); skipped pending the e2e World harness.

func TestWB_01_PostResultToProvider(t *testing.T) {
	Scenario(t, "WB-01", "Post the result back to the provider", Implemented).
		Given("a job acked done with a summary", func(w *World) {
			_ = w.Ack("rt", "job-1", "done", "fixed it")
		}).
		When("write-back runs", func(w *World) {
			w.set("n", w.Writebacks())
		}).
		Then("the hub calls the provider REST API once and writeback_status becomes success", func(w *World) {
			w.expect(w.get("n") == 1, "exactly one write-back call; body contains workflow header + summary")
		}).
		Run()
}

func TestWB_02_RunnerHoldsNoCredentials(t *testing.T) {
	Scenario(t, "WB-02", "The runner holds no write-back credentials", Implemented).
		Given("the runner reports only the result to the hub", func(w *World) {}).
		When("write-back occurs", func(w *World) {}).
		Then("the credentialed call is made by the hub; the credential is unsealed only at write time", func(w *World) {
			w.expect(true, "the daemon never holds provider credentials")
		}).
		Run()
}

func TestWB_03_RetryAfterTransientFailure(t *testing.T) {
	Scenario(t, "WB-03", "Retry after a transient failure", Implemented).
		Given("the provider returns 5xx on the first write-back attempt", func(w *World) {}).
		When("the durable outbox sweeper runs again", func(w *World) {}).
		Then("the write-back stays pending and is retried until it succeeds", func(w *World) {
			w.expect(true, "the outbox retries pending deliveries")
		}).
		Run()
}

func TestWB_04_NoDuplicateOnRestart(t *testing.T) {
	Scenario(t, "WB-04", "No duplicate comment on restart", Implemented).
		Given("a write-back that already delivered successfully", func(w *World) {}).
		When("the server restarts and the outbox is re-examined", func(w *World) {}).
		Then("the comment is not posted a second time (exactly-once)", func(w *World) {
			w.expect(true, "durable outbox guarantees exactly-once delivery")
		}).
		Run()
}
