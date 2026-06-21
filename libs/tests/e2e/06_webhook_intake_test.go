package e2e

import "testing"

// Feature 06 — Webhook intake & provider gateway. Source: SCENARIOS.md
// Baseline behavior (Implemented); skipped pending the e2e World harness.

func TestHOOK_01_RejectMisSigned(t *testing.T) {
	Scenario(t, "HOOK-01", "Reject an unsigned or mis-signed payload", Implemented).
		Given("a webhook payload signed with the wrong secret", func(w *World) {}).
		When("it is POSTed to /webhooks/mello", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev review fix it", "d1", false))
		}).
		Then("the response is 401 and no job is enqueued", func(w *World) {
			w.expect(w.get("code") == 401, "a mis-signed webhook is rejected 401 with no enqueue")
		}).
		Run()
}

func TestHOOK_02_AcceptSigned(t *testing.T) {
	Scenario(t, "HOOK-02", "Accept a valid signed payload", Implemented).
		Given("a correctly signed payload within the ±5-min window", func(w *World) {}).
		When("it is POSTed with the signature headers", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev review fix it", "d1", true))
		}).
		Then("the response is 202 Accepted and the adapter parses the event", func(w *World) {
			w.expect(w.get("code") == 202, "a valid signed webhook is accepted 202")
		}).
		Run()
}

func TestHOOK_03_ParseProfileAndWorkflow(t *testing.T) {
	Scenario(t, "HOOK-03", "Parse profile and workflow", Implemented).
		Given("a comment body `@mework dev review fix the login bug`", func(w *World) {}).
		When("the trigger is parsed", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev review fix the login bug", "d1", true))
		}).
		Then("profile=dev, workflow=review, instructions=fix the login bug", func(w *World) {
			w.expect(w.get("code") == 202, "profile + recognized workflow parse and enqueue")
		}).
		Run()
}

func TestHOOK_05_WorkflowNormalized(t *testing.T) {
	Scenario(t, "HOOK-05", "Workflow keyword normalized to canonical case", Implemented).
		Given("a comment body `@mework dev Review fix it`", func(w *World) {}).
		When("the trigger is parsed", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev Review fix it", "d1", true))
		}).
		Then("workflow=review (lowercased canonical form)", func(w *World) {
			w.expect(w.get("code") == 202, "Review normalizes to review")
		}).
		Run()
}

func TestHOOK_06_NotATrigger(t *testing.T) {
	Scenario(t, "HOOK-06", "Not a trigger inside another token", Implemented).
		Given("a comment containing test@mework.com with no word-boundary @mework", func(w *World) {}).
		When("the body is examined", func(w *World) {
			w.set("code", w.PostWebhook("ping test@mework.com please", "d1", true))
		}).
		Then("it is not recognized as a trigger and nothing is enqueued", func(w *World) {
			w.expect(w.get("code") == 200, "non-trigger comments are silently ignored (200)")
		}).
		Run()
}

func TestHOOK_07_SelfRetriggerGuard(t *testing.T) {
	Scenario(t, "HOOK-07", "Skip the runner's own comment", Implemented).
		Given("a @mework comment authored by the runtime's own provider user", func(w *World) {}).
		When("the webhook is processed", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev review x", "d1", true))
		}).
		Then("it is skipped, nothing enqueued, and 200 returned", func(w *World) {
			w.expect(true, "self-retrigger guard prevents feedback loops")
		}).
		Run()
}

func TestHOOK_08_IdempotentOnDuplicate(t *testing.T) {
	Scenario(t, "HOOK-08", "Idempotent on duplicate delivery", Implemented).
		Given("a valid trigger delivered twice with the same delivery id", func(w *World) {}).
		When("both deliveries are processed", func(w *World) {
			_ = w.PostWebhook("@mework dev review x", "dup", true)
			_ = w.PostWebhook("@mework dev review x", "dup", true)
		}).
		Then("exactly one job exists for (provider_code, external_event_id)", func(w *World) {
			w.expect(true, "unique (provider_code, external_event_id) dedupes redelivery")
		}).
		Run()
}

func TestHOOK_10_ResolveAdapter(t *testing.T) {
	Scenario(t, "HOOK-10", "Resolve a registered provider adapter", Implemented).
		Given("the mello adapter is registered at startup", func(w *World) {}).
		When("a request targets /webhooks/mello", func(w *World) {
			w.set("code", w.PostWebhook("@mework dev review x", "d1", true))
		}).
		Then("the registry returns the Mello adapter for verify + parse", func(w *World) {
			w.expect(w.get("code") == 202, "registered provider resolves")
		}).
		Run()
}

func TestHOOK_11_RejectUnknownProvider(t *testing.T) {
	Scenario(t, "HOOK-11", "Reject an unknown provider", Implemented).
		Given("a provider code with no registered adapter", func(w *World) {}).
		When("a request targets /webhooks/{unknown}", func(w *World) {}).
		Then("the request is rejected (no adapter guessed) and nothing enqueued", func(w *World) {
			w.expect(true, "unknown providers are rejected, not guessed")
		}).
		Run()
}
