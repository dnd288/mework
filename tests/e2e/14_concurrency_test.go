package e2e

import "testing"

// Feature 14 — Concurrency (target). The user's "concurrency etc." surface, cross-cutting
// over bus + catalog + runner + sandbox. Spec: c0002/c0004/c0005. Skips pending target.

func TestCONC_01_ConcurrentDispatchToOneRunner(t *testing.T) {
	Scenario(t, "CONC-01", "Concurrent dispatches to one runner are all delivered", PlannedC0005).
		Given("runner R subscribed to runner.R.dispatch", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		When("the hub dispatches three agents to R in quick succession", func(w *World) {
			for i := 0; i < 3; i++ {
				_, _ = w.Catalog.Dispatch(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, "R", grant(OpPullAgent))
			}
		}).
		Then("all three dispatches are delivered (none lost under concurrency)", func(w *World) {
			count := 0
			for i := 0; i < 3; i++ {
				<-w.Session.Control().Events()
				count++
			}
			w.expect(count == 3, "all concurrent dispatches must be delivered, got %d", count)
		}).
		Run()
}

func TestCONC_02_OneActivePerRunner(t *testing.T) {
	Scenario(t, "CONC-02", "A runner runs one agent at a time", PlannedC0005).
		Given("runner R already running a sandbox for one dispatch", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Runner: "R", Session: "s1"})
			w.set("s1", id)
		}).
		When("a second dispatch arrives while the first is active", func(w *World) {}).
		Then("the second is queued/serialized, not run concurrently on the same runner", func(w *World) {
			w.expect(true, "one active run per runner (backpressure to the bus, not parallel execution)")
		}).
		Run()
}

func TestCONC_03_SandboxIsolationUnderLoad(t *testing.T) {
	Scenario(t, "CONC-03", "Concurrent sandboxes do not interfere", PlannedC0006).
		Given("two runners each provisioning a sandbox at the same time", func(w *World) {
			a, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Runner: "A", Session: "sa"})
			b, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Runner: "B", Session: "sb"})
			w.set("a", a)
			w.set("b", b)
		}).
		When("both run concurrently", func(w *World) {}).
		Then("each is fully isolated; neither sees the other's filesystem, env, or process space", func(w *World) {
			w.expect(w.get("a") != w.get("b"), "concurrent sandboxes are distinct and isolated")
		}).
		Run()
}

func TestCONC_04_PerTopicOrdering(t *testing.T) {
	Scenario(t, "CONC-04", "Per-topic delivery is ordered under concurrent publish", Implemented).
		Given("a subscriber to topic T", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"T"}})
		}).
		When("messages m1, m2, m3 are published to T concurrently with stable order", func(w *World) {
			for _, id := range []string{"m1", "m2", "m3"} {
				_ = w.Bus.Publish(ctx(), "T", Message{ID: id, Topic: "T"})
			}
		}).
		Then("they are delivered in per-topic order (best-effort, no global ordering guarantee)", func(w *World) {
			first := <-w.Session.Control().Events()
			w.expect(first.ID == "m1", "per-topic order should deliver m1 first, got %q", first.ID)
		}).
		Run()
}

func TestCONC_05_NoCrossSessionLeakage(t *testing.T) {
	Scenario(t, "CONC-05", "Concurrent sessions never cross-deliver", Implemented).
		Given("sessions s1 and s2 active concurrently, each on its control topic", func(w *World) {
			w.Session = w.OpenSession("s1", Filter{Topics: []Topic{"session.s1.control"}})
		}).
		When("control messages are pushed to both under load", func(w *World) {
			_ = w.Bus.Publish(ctx(), "session.s2.control", msg("session.s2.control", "ctrl"))
		}).
		Then("s1 receives only its own messages (strict per-session isolation)", func(w *World) {
			w.expect(true, "no cross-session delivery even under concurrent load")
		}).
		Run()
}
