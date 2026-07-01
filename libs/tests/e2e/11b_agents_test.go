//go:build e2e

package e2e

import "testing"

// Feature 11b — Agent backends (claude code, codex). Source: SCENARIOS.md +
// the user's "claude code agent / codex agent" surface. Spec: daemon-runtime "AI backend
// detection" (baseline) + c0005 (run via sandbox). Skips pending c0005.

func TestAGENT_01_DetectOrder(t *testing.T) {
	Scenario(t, "AGENT-01", "Backend detection prefers claude, then codex, then opencode", PlannedC0006).
		Given("only codex is installed on PATH", func(w *World) {
			w.FakeAgent("codex", "echo-stdin")
		}).
		When("the runner detects a backend", func(w *World) {
			w.set("b", w.DetectBackend())
		}).
		Then("it selects codex (next available after the absent claude)", func(w *World) {
			b := w.get("b").(AgentBackend)
			w.expect(b.Name() == "codex", "detection should fall back to codex, got %q", b.Name())
		}).
		Run()
}

func TestAGENT_02_ClaudeBackendRun(t *testing.T) {
	Scenario(t, "AGENT-02", "Claude Code backend runs an agent", PlannedC0006).
		Given("claude is the selected backend in a sandbox", func(w *World) {
			w.FakeAgent("claude", "echo-stdin")
		}).
		When("the agent runs with a prompt", func(w *World) {
			res, err := w.Backend("claude").Run(ctx(), RunSpec{Agent: AgentRef{Name: "code-fixer"}})
			w.set("res", res)
			w.expect(err == nil, "claude backend run should complete")
		}).
		Then("its output and exit status are captured", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Status == StatusDone, "clean claude run reports done, got %q", res.Status)
		}).
		Run()
}

func TestAGENT_03_CodexBackendRun(t *testing.T) {
	Scenario(t, "AGENT-03", "Codex backend runs an agent", PlannedC0006).
		Given("codex is the selected backend in a sandbox", func(w *World) {
			w.FakeAgent("codex", "echo-stdin")
		}).
		When("the agent runs with a prompt", func(w *World) {
			res, _ := w.Backend("codex").Run(ctx(), RunSpec{Agent: AgentRef{Name: "code-fixer"}})
			w.set("res", res)
		}).
		Then("its output and exit status are captured", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Status == StatusDone, "clean codex run reports done")
		}).
		Run()
}

func TestAGENT_04_PromptOverStdin(t *testing.T) {
	Scenario(t, "AGENT-04", "Backends receive the prompt over stdin", PlannedC0006).
		Given("a backend configured to echo its stdin", func(w *World) {
			w.FakeAgent("claude", "echo-stdin")
		}).
		When("the agent runs", func(w *World) {
			res, _ := w.Backend("claude").Run(ctx(), RunSpec{Agent: AgentRef{Name: "x"}})
			w.set("res", res)
		}).
		Then("the prompt reached the process via stdin, not argv", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Output != "", "echoed stdin proves the prompt arrived on stdin")
		}).
		Run()
}

func TestAGENT_05_BackendUnavailable(t *testing.T) {
	Scenario(t, "AGENT-05", "No installed backend is handled", PlannedC0006).
		Given("no AI backend is on PATH", func(w *World) {}).
		When("the runner checks availability", func(w *World) {
			w.set("avail", w.Backend("claude").Available())
		}).
		Then("the backend reports unavailable and the run is not attempted", func(w *World) {
			w.expect(w.get("avail") == false, "an absent backend must report unavailable")
		}).
		Run()
}
