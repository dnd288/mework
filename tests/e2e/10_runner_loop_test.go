package e2e

import "testing"

// Feature 10 — Runner loop (enrolled SSE, target). Source: SCENARIOS.md
// Spec: openspec/changes/c0004-agent-runner. Skips pending c0004.

func TestLOOP_01_RunnerComesOnline(t *testing.T) {
	Scenario(t, "LOOP-01", "Runner comes online (presence)", PlannedC0005).
		Given("an enrolled runner R", func(w *World) {}).
		When("R opens its SSE subscription", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		Then("the hub marks R present/online and eligible for dispatches", func(w *World) {
			online, _ := w.Registry.Presence(ctx(), "R")
			w.expect(online, "R should be marked online once subscribed")
		}).
		Run()
}

func TestLOOP_02_NoIntervalPolling(t *testing.T) {
	Scenario(t, "LOOP-02", "No interval polling when idle", PlannedC0005).
		Given("R is online and idle (no dispatches)", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		When("time passes", func(w *World) {}).
		Then("R holds its open SSE subscription and issues no periodic claim/poll", func(w *World) {
			w.expect(true, "an enrolled runner must not poll a claim endpoint on an interval")
		}).
		Run()
}

func TestLOOP_03_ReceiveDispatchByPush(t *testing.T) {
	Scenario(t, "LOOP-03", "Receive a dispatch by push", PlannedC0005).
		Given("R is subscribed", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		When("the hub dispatches code-fixer@1.2.0 to R's topic", func(w *World) {
			_, _ = w.Catalog.Dispatch(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, "R", grant(OpPullAgent))
		}).
		Then("R receives it over the SSE stream without polling", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Kind == "dispatch", "R should receive the dispatch by push")
		}).
		Run()
}

func TestLOOP_04_SuccessfulDispatchLifecycle(t *testing.T) {
	Scenario(t, "LOOP-04", "Successful dispatch lifecycle", PlannedC0005).
		Given("R received a dispatch for code-fixer@1.2.0", func(w *World) {}).
		When("R processes it", func(w *World) {
			w.expect(w.StartRunner() == nil, "runner loop should start")
		}).
		Then("R pulls the exact version, runs it in a sandbox, posts the result, and acks the dispatch", func(w *World) {
			w.expect(true, "pull → run → report → ack; not redelivered after ack")
		}).
		Run()
}

func TestLOOP_05_FailedRunReported(t *testing.T) {
	Scenario(t, "LOOP-05", "Failed run is reported, not dropped", PlannedC0005).
		Given("a dispatch whose agent exits non-zero", func(w *World) {
			w.FakeAgent("claude", "exit-nonzero")
		}).
		When("R runs it", func(w *World) {}).
		Then("R reports failed with a summary and acks the dispatch", func(w *World) {
			w.expect(true, "a failed run is reported failed and acked (not left for redelivery)")
		}).
		Run()
}

func TestLOOP_06_OperationWithinGrantProceeds(t *testing.T) {
	Scenario(t, "LOOP-06", "Operation within the grant proceeds", PlannedC0005).
		Given("a dispatch whose grant covers the attempted operation", func(w *World) {
			w.Grant = grant(OpRepoRead)
		}).
		When("R mediates that operation", func(w *World) {}).
		Then("R permits it", func(w *World) {
			w.expect(w.Grants.Permits(w.Grant, OpRepoRead), "an in-grant operation must be permitted")
		}).
		Run()
}

func TestLOOP_07_OperationOutsideGrantRefused(t *testing.T) {
	Scenario(t, "LOOP-07", "Operation outside the grant is refused locally", PlannedC0005).
		Given("a dispatch whose grant does not cover an attempted operation", func(w *World) {
			w.Grant = grant(OpRepoRead)
		}).
		When("R mediates that operation", func(w *World) {}).
		Then("R refuses it locally and reports the refusal; it cannot widen its own grant", func(w *World) {
			w.expect(!w.Grants.Permits(w.Grant, OpNetwork), "an out-of-grant op must be refused locally")
		}).
		Run()
}

func TestLOOP_08_ReconnectResume(t *testing.T) {
	Scenario(t, "LOOP-08", "Reconnect with jittered backoff and resume", PlannedC0005).
		Given("R's SSE connection dropped after processing event id 7", func(w *World) {
			w.set("lastID", "7")
		}).
		When("R reconnects", func(w *World) {
			sub := w.Subscribe(Filter{Topics: []Topic{"runner.R.dispatch"}}, "7")
			w.set("sub", sub)
		}).
		Then("it resumes with Last-Event-ID and misses no dispatch", func(w *World) {
			sub := w.get("sub").(Subscription)
			ev := <-sub.Events()
			w.expect(ev.ID > "7", "resume should deliver only newer events")
		}).
		Run()
}

func TestLOOP_09_CrashRecovery(t *testing.T) {
	Scenario(t, "LOOP-09", "Runner restarts and recovers in-flight bookkeeping", PlannedC0005).
		Given("R crashed mid-run with one dispatch unacked", func(w *World) {}).
		When("R restarts with its persisted runner identity", func(w *World) {
			_, err := w.EnrollRunner("") // identity already persisted; no re-enroll needed
			w.set("err", err)
		}).
		Then("the unacked dispatch is redelivered and R resumes the pull→run→report loop", func(w *World) {
			w.expect(true, "crash recovery: unacked work is redelivered, not lost or duplicated")
		}).
		Run()
}
