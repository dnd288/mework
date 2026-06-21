package e2e

import "testing"

// Feature 11 — Sandbox execution (target). Source: SCENARIOS.md
// Spec: openspec/changes/c0005-sandbox-runtime. Skips pending c0005.
// Covers driver interface, local + docker, lifecycle, crash handling, resource limits.

func TestSBX_01_RunThroughDriverInterface(t *testing.T) {
	Scenario(t, "SBX-01", "Run an agent through the driver interface", PlannedC0006).
		Given("a RunSpec (agent ref, workdir, env scope, limits, timeout)", func(w *World) {
			w.set("spec", RunSpec{Agent: AgentRef{Name: "code-fixer", Version: "1.2.0"}, Driver: DriverLocal})
		}).
		When("the runner calls Driver.Run via create→start→exec→stop→destroy", func(w *World) {
			res, err := w.Driver(DriverLocal).Run(ctx(), w.get("spec").(RunSpec))
			w.set("res", res)
			w.expect(err == nil, "driver run should complete")
		}).
		Then("captured stdout/stderr and exit status are returned", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Status == StatusDone, "a clean run reports done, got %q", res.Status)
		}).
		Run()
}

func TestSBX_02_PromptOnStdinNeverArgv(t *testing.T) {
	Scenario(t, "SBX-02", "Prompt is never placed on the command line", PlannedC0006).
		Given("attacker-controllable prompt content from a ticket/agent", func(w *World) {}).
		When("the agent runs (driver streams the prompt)", func(w *World) {
			_, _ = w.Driver(DriverLocal).Run(ctx(), RunSpec{Agent: AgentRef{Name: "x"}})
		}).
		Then("the prompt is delivered on stdin and never appears in argv", func(w *World) {
			w.expect(true, "stdin-not-argv is the command-injection control, preserved by every driver")
		}).
		Run()
}

func TestSBX_03_SelectLocalDriver(t *testing.T) {
	Scenario(t, "SBX-03", "Select the local driver", PlannedC0006).
		Given("a dispatch/config selecting the local driver", func(w *World) {}).
		When("the agent runs", func(w *World) {
			w.expect(w.Driver(DriverLocal).Kind() == DriverLocal, "local driver selected")
		}).
		Then("it runs as a host subprocess in an isolated workdir; documents no host isolation", func(w *World) {
			w.expect(true, "local = today's behavior formalized; trusted use only")
		}).
		Run()
}

func TestSBX_04_SelectDockerDriver(t *testing.T) {
	Scenario(t, "SBX-04", "Select the docker driver", PlannedC0006).
		Given("a dispatch/config selecting the docker driver", func(w *World) {}).
		When("the agent runs", func(w *World) {
			w.expect(w.Driver(DriverDocker).Kind() == DriverDocker, "docker driver selected")
		}).
		Then("it runs in a container with only the provisioned workdir mounted and scoped network/env", func(w *World) {
			w.expect(true, "container-per-agent isolation")
		}).
		Run()
}

func TestSBX_05_AddDriverWithoutChangingCallers(t *testing.T) {
	Scenario(t, "SBX-05", "Add a new driver without changing callers", PlannedC0006).
		Given("a new driver implementing the SandboxDriver interface", func(w *World) {}).
		When("it is registered", func(w *World) {}).
		Then("existing callers use it unchanged; a local-only build pulls in no Docker dependency", func(w *World) {
			w.expect(true, "driver-gated deps; selection is the only difference")
		}).
		Run()
}

func TestSBX_06_IsolationBetweenRuns(t *testing.T) {
	Scenario(t, "SBX-06", "Isolation between runs", PlannedC0006).
		Given("two agents dispatched to the same runner", func(w *World) {
			s1, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			s2, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s2"})
			w.set("s1", s1)
			w.set("s2", s2)
		}).
		When("both run", func(w *World) {}).
		Then("each runs in its own sandbox with no shared filesystem or process space", func(w *World) {
			w.expect(w.get("s1") != w.get("s2"), "each run gets a distinct sandbox")
		}).
		Run()
}

func TestSBX_07_TornDownAfterRun(t *testing.T) {
	Scenario(t, "SBX-07", "Sandbox is torn down after the run", PlannedC0006).
		Given("an agent run reaches a terminal state", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			w.set("id", id)
		}).
		When("the run completes", func(w *World) {
			w.expect(w.SandboxMgr.Destroy(ctx(), w.get("id").(SandboxID)) == nil, "destroy should succeed")
		}).
		Then("the sandbox is stopped and destroyed, resources released (even on failure)", func(w *World) {
			st, _ := w.SandboxMgr.State(ctx(), w.get("id").(SandboxID))
			w.expect(st == SandboxDestroyed, "sandbox should be destroyed, got %q", st)
		}).
		Run()
}

func TestSBX_08_DockerConfinesHostPaths(t *testing.T) {
	Scenario(t, "SBX-08", "Docker driver confines host paths", PlannedC0006).
		Given("the docker driver", func(w *World) {}).
		When("the agent attempts to read/write host paths outside those provisioned", func(w *World) {}).
		Then("access is denied by the container boundary", func(w *World) {
			w.expect(true, "only the provisioned workdir is reachable")
		}).
		Run()
}

func TestSBX_09_ResourceLimitTerminatesRunaway(t *testing.T) {
	Scenario(t, "SBX-09", "Resource limit terminates a runaway agent", PlannedC0006).
		Given("an agent that sleeps past the wall-clock limit (default 30m)", func(w *World) {
			w.set("spec", RunSpec{Agent: AgentRef{Name: "x"}, Driver: DriverDocker})
		}).
		When("the sandboxed run exceeds the limit", func(w *World) {
			res, _ := w.Driver(DriverDocker).Run(ctx(), w.get("spec").(RunSpec))
			w.set("res", res)
		}).
		Then("the sandbox terminates the run and the dispatch is reported failed", func(w *World) {
			res := w.get("res").(Result)
			w.expect(res.Status == StatusFailed, "a limit breach reports failed, got %q", res.Status)
		}).
		Run()
}

func TestSBX_10_PullSandboxImage(t *testing.T) {
	Scenario(t, "SBX-10", "Pull an image-form agent into a sandbox", PlannedC0006).
		Given("an image-form agent version dispatched to the docker driver", func(w *World) {}).
		When("the manager provisions the sandbox", func(w *World) {
			id, err := w.SandboxMgr.Provision(ctx(), Dispatch{Agent: AgentRef{Name: "img", Version: "1.0.0"}})
			w.set("id", id)
			w.expect(err == nil, "provision should pull the image and create the sandbox")
		}).
		Then("the referenced image is materialized for the run", func(w *World) {
			st, _ := w.SandboxMgr.State(ctx(), w.get("id").(SandboxID))
			w.expect(st == SandboxRunning, "sandbox should be running after provision, got %q", st)
		}).
		Run()
}

func TestSBX_11_ManageInspectSandbox(t *testing.T) {
	Scenario(t, "SBX-11", "Inspect a running sandbox's state", PlannedC0006).
		Given("a provisioned, running sandbox", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			w.set("id", id)
		}).
		When("its state is queried", func(w *World) {
			st, _ := w.SandboxMgr.State(ctx(), w.get("id").(SandboxID))
			w.set("st", st)
		}).
		Then("the manager reports its lifecycle state", func(w *World) {
			w.expect(w.get("st").(SandboxState) == SandboxRunning, "state should be observable")
		}).
		Run()
}

func TestCRASH_01_SandboxCrashReportedFailed(t *testing.T) {
	Scenario(t, "CRASH-01", "A crashed sandbox is reported failed", PlannedC0006).
		Given("a running sandbox whose process crashes mid-run", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			w.set("id", id)
		}).
		When("the crash is detected", func(w *World) {
			_ = w.SandboxMgr.OnCrash(ctx(), w.get("id").(SandboxID), func(r Result) { w.set("res", r) })
		}).
		Then("the dispatch is reported failed with a crash summary", func(w *World) {
			st, _ := w.SandboxMgr.State(ctx(), w.get("id").(SandboxID))
			w.expect(st == SandboxCrashed, "a crashed sandbox is observable as crashed, got %q", st)
		}).
		Run()
}

func TestCRASH_02_CleanupAfterCrash(t *testing.T) {
	Scenario(t, "CRASH-02", "Resources are released after a crash", PlannedC0006).
		Given("a sandbox that crashed", func(w *World) {
			id, _ := w.SandboxMgr.Provision(ctx(), Dispatch{Session: "s1"})
			w.set("id", id)
		}).
		When("the manager reconciles the crashed sandbox", func(w *World) {
			_ = w.SandboxMgr.Destroy(ctx(), w.get("id").(SandboxID))
		}).
		Then("it is destroyed and its resources released (no leak)", func(w *World) {
			st, _ := w.SandboxMgr.State(ctx(), w.get("id").(SandboxID))
			w.expect(st == SandboxDestroyed, "cleanup must run even after a crash")
		}).
		Run()
}

func TestCRASH_03_RunnerSurvivesSandboxCrash(t *testing.T) {
	Scenario(t, "CRASH-03", "Runner survives a sandbox crash and keeps serving", PlannedC0006).
		Given("a runner with one sandbox that crashes", func(w *World) {}).
		When("the crash propagates to the runner", func(w *World) {}).
		Then("the runner reports that dispatch failed and remains online for the next dispatch", func(w *World) {
			w.expect(true, "one sandbox crashing must not take down the runner")
		}).
		Run()
}
