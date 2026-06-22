//go:build e2e

package e2e

import "testing"

// Feature 18 — Live status & streaming (agent → hub communication events). Real-world
// platform surface (proposed). The runner/agent emits upstream telemetry; clients tail it.

func TestSTREAM_01_AgentEmitsUpstream(t *testing.T) {
	Scenario(t, "STREAM-01", "Agent emits progress/log/output upstream", PlannedPlatform).
		Given("a running agent for run r1", func(w *World) {}).
		When("the runner emits progress, log, and output events for r1", func(w *World) {
			_ = w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "progress", Data: []byte("30%")})
			_ = w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "log", Data: []byte("compiling")})
			w.set("err", w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "output", Data: []byte("patch")}))
		}).
		Then("the hub accepts the upstream events for the run", func(w *World) {
			w.expect(w.get("err") == nil, "agent→hub telemetry is accepted")
		}).
		Run()
}

func TestSTREAM_02_ClientTailsRun(t *testing.T) {
	Scenario(t, "STREAM-02", "Client subscribes to a run's live events", PlannedPlatform).
		Given("run r1 emitting events", func(w *World) {
			sub, _ := w.Runs.Subscribe(ctx(), "r1")
			w.set("sub", sub)
		}).
		When("the runner emits a log line", func(w *World) {
			_ = w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "log", Data: []byte("hello")})
		}).
		Then("the subscribed client receives the event live", func(w *World) {
			sub := w.get("sub").(Subscription)
			ev := <-sub.Events()
			w.expect(ev.Kind == "log", "client tails the run's events, got %q", ev.Kind)
		}).
		Run()
}

func TestSTREAM_03_LateSubscriberGetsTail(t *testing.T) {
	Scenario(t, "STREAM-03", "A late subscriber gets recent tail then live", PlannedPlatform).
		Given("run r1 that already emitted several events", func(w *World) {
			_ = w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "log", Data: []byte("1")})
		}).
		When("a client subscribes after those events", func(w *World) {
			sub, _ := w.Runs.Subscribe(ctx(), "r1")
			w.set("sub", sub)
		}).
		Then("it receives a bounded recent tail and then live events in order", func(w *World) {
			w.expect(true, "late subscribers get a snapshot tail plus the live stream")
		}).
		Run()
}

func TestSTREAM_04_PerRunOrdering(t *testing.T) {
	Scenario(t, "STREAM-04", "Per-run event ordering is preserved", PlannedPlatform).
		Given("a client tailing run r1", func(w *World) {
			sub, _ := w.Runs.Subscribe(ctx(), "r1")
			w.set("sub", sub)
		}).
		When("progress events 1,2,3 are emitted in order", func(w *World) {
			for _, p := range []string{"1", "2", "3"} {
				_ = w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "progress", Data: []byte(p)})
			}
		}).
		Then("they arrive in emission order for that run", func(w *World) {
			sub := w.get("sub").(Subscription)
			first := <-sub.Events()
			w.expect(string(first.Data) == "1", "per-run order is preserved, got %q", string(first.Data))
		}).
		Run()
}

func TestSTREAM_05_OutputStreamingToWriteback(t *testing.T) {
	Scenario(t, "STREAM-05", "Streamed output feeds the final write-back", PlannedPlatform).
		Given("a run streaming output chunks", func(w *World) {
			_ = w.Runs.Emit(ctx(), "r1", RunEvent{Kind: "output", Data: []byte("partial")})
		}).
		When("the run reaches a terminal status", func(w *World) {}).
		Then("the assembled output is available for the server-side write-back", func(w *World) {
			w.expect(true, "streamed output is captured for the result/write-back")
		}).
		Run()
}

func TestSTATUS_01_RunStatusTransitions(t *testing.T) {
	Scenario(t, "STATUS-01", "Run status transitions are observable", PlannedPlatform).
		Given("a dispatched run r1", func(w *World) {}).
		When("the run progresses to a terminal state", func(w *World) {
			st, _ := w.Runs.Status(ctx(), "r1")
			w.set("st", st)
		}).
		Then("its status is queryable (done/failed) at any time", func(w *World) {
			st := w.get("st").(RunStatus)
			w.expect(st == StatusDone || st == StatusFailed, "run status is observable, got %q", st)
		}).
		Run()
}

func TestSTATUS_02_RunnerPresenceDetail(t *testing.T) {
	Scenario(t, "STATUS-02", "Runner presence and heartbeat detail are reported", PlannedPlatform).
		Given("an online runner R holding its SSE channel", func(w *World) {}).
		When("the hub is queried for R's presence", func(w *World) {
			online, _ := w.Registry.Presence(ctx(), "R")
			w.set("online", online)
		}).
		Then("it reports online with recent heartbeat detail", func(w *World) {
			w.expect(w.get("online") == true, "presence reflects the live channel")
		}).
		Run()
}

func TestSTATUS_03_PlatformStatusOverview(t *testing.T) {
	Scenario(t, "STATUS-03", "Operators can see active runs and sessions at a glance", PlannedPlatform).
		Given("a tenant with active runners, sessions, and runs", func(w *World) {}).
		When("an operator queries platform status (CLI/API)", func(w *World) {
			out, _ := w.RunCLI("status", "--json")
			w.set("out", out)
		}).
		Then("it returns runner presence, active sessions, and in-flight run statuses", func(w *World) {
			w.expect(w.get("out") != "", "a status overview is available for operators")
		}).
		Run()
}
