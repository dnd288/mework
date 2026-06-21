package e2e

import "testing"

// Feature 19 — Cancellation. Real-world platform surface (proposed). Cancel a running
// run/dispatch/scheduled job; cancellation propagates to the sandbox and is terminal.

func TestCANCEL_01_CancelRunningRun(t *testing.T) {
	Scenario(t, "CANCEL-01", "Cancel a running run (graceful then forced)", PlannedPlatform).
		Given("a run r1 in progress", func(w *World) {}).
		When("an operator cancels it gracefully, then forces if it does not stop", func(w *World) {
			_ = w.Runs.Cancel(ctx(), "r1", false)
			w.set("err", w.Runs.Cancel(ctx(), "r1", true))
		}).
		Then("the run stops and is reported failed/canceled", func(w *World) {
			w.expect(w.get("err") == nil, "graceful→forced cancel terminates the run")
		}).
		Run()
}

func TestCANCEL_02_PropagatesToSandbox(t *testing.T) {
	Scenario(t, "CANCEL-02", "Cancel propagates to the sandbox", PlannedPlatform).
		Given("a run executing inside a sandbox", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			w.set("id", id)
		}).
		When("the run is canceled", func(w *World) {
			_ = w.Runs.Cancel(ctx(), "r1", false)
		}).
		Then("the sandbox is stopped/destroyed and resources released", func(w *World) {
			st, _ := w.SandboxMgr.State(ctx(), w.get("id").(SandboxID))
			w.expect(st == SandboxDestroyed, "cancellation tears down the sandbox, got %q", st)
		}).
		Run()
}

func TestCANCEL_03_CancelScheduledRun(t *testing.T) {
	Scenario(t, "CANCEL-03", "Cancel a scheduled run before it fires", PlannedPlatform).
		Given("a one-shot run-at schedule that has not yet fired", func(w *World) {
			id, _ := w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleAt, Target: "R"})
			w.set("id", id)
		}).
		When("the schedule is canceled before its fire time", func(w *World) {
			w.set("err", w.Scheduler.Cancel(ctx(), w.get("id").(ScheduleID)))
		}).
		Then("it never dispatches", func(w *World) {
			w.expect(w.get("err") == nil, "canceling a pending schedule prevents the dispatch")
		}).
		Run()
}

func TestCANCEL_04_IdempotentTerminal(t *testing.T) {
	Scenario(t, "CANCEL-04", "Cancel is idempotent and terminal", PlannedPlatform).
		Given("a run that has already been canceled", func(w *World) {
			_ = w.Runs.Cancel(ctx(), "r1", false)
		}).
		When("cancel is issued again", func(w *World) {
			w.set("err", w.Runs.Cancel(ctx(), "r1", false))
		}).
		Then("it is a no-op success and the run cannot resume", func(w *World) {
			w.expect(w.get("err") == nil, "repeated cancel is an idempotent no-op; canceled is terminal")
		}).
		Run()
}
