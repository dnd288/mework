## 1. Driver interface

- [ ] 1.1 Define `Sandbox`/`Driver` interface (create/start/exec/stop/destroy, stdin prompt, captured output, exit status) in `sandbox/runtime` (was `sandbox/engine/local`)
- [ ] 1.2 Define a `RunSpec` (agent ref/command, workdir, env scope, resource limits, timeout) and a `Result` type
- [ ] 1.3 Refactor the former `agentrun` exec seam (`exec.CommandContext`) to call `Driver.Run` instead

## 2. Local driver

- [ ] 2.1 Implement the `local` driver preserving current behavior (host subprocess, isolated workdir, stdin prompt, 30m default timeout)
- [ ] 2.2 Document that `local` provides no host isolation (trusted use only)

## 3. Docker driver

- [ ] 3.1 Implement the `docker` driver: container per agent, mount only the provisioned workdir, scope network/env
- [ ] 3.2 Apply CPU/memory limits and the wall-clock timeout; stream prompt to container stdin; capture output
- [ ] 3.3 Gate the Docker client dependency behind the driver so `local`-only builds add no dep

## 4. Selection & lifecycle

- [ ] 4.1 Select the driver per dispatch/config (`sandbox/engine/{local,docker}`); default and override rules
- [ ] 4.2 Enforce one agent per sandbox; create on run, destroy on terminal state (guarantee cleanup on failure)

## 5. Validation

- [ ] 5.1 Tests: local driver parity with current behavior; docker driver isolation + limits + timeout; stdin-not-argv; teardown after run
- [ ] 5.2 `openspec validate --change sandbox-runtime --strict`
- [ ] 5.3 e2e pointer: flip `tests/e2e/11_sandbox_test.go` from Skip to Green for SBX-01..09 and CRASH-01..02 (crashed sandbox reported failed, resources released); flip `tests/e2e/11b_agents_test.go` from Skip to Green for AGENT-01..05; flip `tests/e2e/14_concurrency_test.go` CONC-03 (concurrent sandboxes do not interfere); CRASH-03 (runner survives a sandbox crash) is asserted in `tests/e2e/10_runner_loop_test.go`. The MODIFIED daemon-runtime requirement (stdin-not-argv today) is exercised by `tests/e2e/05_daemon_test.go` DAEMON-09 and must remain green.
