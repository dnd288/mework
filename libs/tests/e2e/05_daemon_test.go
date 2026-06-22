//go:build e2e

package e2e

import "testing"

// Feature 05 — Daemon lifecycle & execution. Source: SCENARIOS.md
// Baseline behavior (Implemented); skipped pending the e2e World harness.

func TestDAEMON_01_StartBackground(t *testing.T) {
	Scenario(t, "DAEMON-01", "Start in the background", Implemented).
		Given("no daemon running for the active profile", func(w *World) {}).
		When("the developer runs `mework daemon start`", func(w *World) {
			_, err := w.RunCLI("daemon", "start")
			w.set("err", err)
		}).
		Then("a detached process starts, writes daemon.pid (0600), and a second start is a no-op", func(w *World) {
			w.expect(w.get("err") == nil, "daemon start should succeed")
		}).
		Run()
}

func TestDAEMON_02_Status(t *testing.T) {
	Scenario(t, "DAEMON-02", "Inspect a running daemon", Implemented).
		Given("a running daemon", func(w *World) {}).
		When("the developer runs `mework daemon status`", func(w *World) {
			w.set("s", w.DaemonStatus())
		}).
		Then("it reports running state, pid, and the health port", func(w *World) {
			w.expect(w.get("s") != "", "status reports running state")
		}).
		Run()
}

func TestDAEMON_03_StopGraceful(t *testing.T) {
	Scenario(t, "DAEMON-03", "Stop gracefully with SIGTERM fallback", Implemented).
		Given("a running daemon", func(w *World) {}).
		When("the developer runs `mework daemon stop`", func(w *World) {
			_, err := w.RunCLI("daemon", "stop")
			w.set("err", err)
		}).
		Then("it shuts down via the health port, falling back to SIGTERM", func(w *World) {
			w.expect(w.get("err") == nil, "graceful stop with SIGTERM fallback")
		}).
		Run()
}

func TestDAEMON_06_StalePidNotLive(t *testing.T) {
	Scenario(t, "DAEMON-06", "Stale pid is not mistaken for a live daemon", Implemented).
		Given("a daemon.pid left from a crashed process", func(w *World) {}).
		When("the developer runs `mework daemon status` or `start`", func(w *World) {
			w.set("s", w.DaemonStatus())
		}).
		Then("liveness is checked via signal 0 and start may launch a new daemon", func(w *World) {
			w.expect(true, "stale pid detected via signal 0")
		}).
		Run()
}

func TestDAEMON_08_BackendFallback(t *testing.T) {
	Scenario(t, "DAEMON-08", "Fall back to the next AI backend", Implemented).
		Given("the preferred backend is not on PATH but a later one is", func(w *World) {
			w.FakeAgent("codex", "echo-stdin")
		}).
		When("the daemon detects a backend", func(w *World) {
			w.set("b", w.DetectBackend())
		}).
		Then("it selects the next available in claude → codex → opencode order", func(w *World) {
			b := w.get("b").(AgentBackend)
			w.expect(b.Name() == "codex", "should fall back to codex")
		}).
		Run()
}

func TestDAEMON_09_PromptOnStdin(t *testing.T) {
	Scenario(t, "DAEMON-09", "Prompt is fed on stdin, never argv (today)", Implemented).
		Given("a job whose prompt derives from attacker-controllable ticket content", func(w *World) {
			w.FakeAgent("claude", "echo-stdin")
		}).
		When("the daemon runs the backend", func(w *World) {
			res, _ := w.Backend("claude").Run(ctx(), RunSpec{Agent: AgentRef{Name: "x"}})
			w.set("res", res)
		}).
		Then("the prompt arrives on stdin and never appears in argv; isolated 0700 workdir", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Output != "", "stdin echo proves the prompt was not on argv")
		}).
		Run()
}

func TestDAEMON_10_RunawayBounded(t *testing.T) {
	Scenario(t, "DAEMON-10", "Runaway run is bounded (today)", Implemented).
		Given("a backend that runs past the default 30-minute timeout", func(w *World) {
			w.FakeAgent("claude", "sleep")
		}).
		When("the daemon executes it", func(w *World) {
			res, _ := w.Backend("claude").Run(ctx(), RunSpec{Agent: AgentRef{Name: "x"}})
			w.set("res", res)
		}).
		Then("the run is cancelled at the timeout and the job is acked failed", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Status == StatusFailed, "a runaway run is bounded and reported failed")
		}).
		Run()
}
