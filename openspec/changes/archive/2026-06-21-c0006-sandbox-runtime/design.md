## Context

Agent execution today is a single `exec.CommandContext(runCtx, b.Path)` with the
prompt on stdin and a per-job host directory (`sandbox/engine/local/runner.go:37`,
`WorkDir` at `runner.go:59`). The only isolation is a `0700` folder; the agent runs
as the daemon's OS user with full host access. To run arbitrary permitted agents
dispatched from a hub, execution must be isolatable. `runner.go:37` is the single
seam where a driver dispatch replaces the bare exec.

## Goals / Non-Goals

**Goals:**
- A driver interface so execution does not depend on a concrete runtime.
- At least `local` (current behavior) and `docker` (container per agent) drivers.
- One agent per sandbox; lifecycle bound to the run.
- Resource limits + timeout; host isolation for isolating drivers.
- Preserve the stdin-not-argv invariant.

**Non-Goals:**
- Deciding the single "best" isolation tech — the interface keeps it open
  (gVisor/Firecracker/Podman/remote sandbox services can be added as drivers).
- The pull/dispatch/grant flow (`agent-catalog`, `agent-runner`).
- Image building/registry for image-form agents (catalog concern).

## Decisions

- **Interface at the `runner.go:37` seam.** Replace the bare exec with
  `Driver.Run(ctx, spec) -> result`; keep `RunResult`/`WorkDir` semantics for the
  `local` driver so current behavior is unchanged.
- **`local` is honest about isolation.** It is the trusted-use default and is
  documented as providing *no* host isolation — only a working directory. Untrusted
  or hub-dispatched agents should select an isolating driver.
- **`docker` driver = container per agent.** Mounts only the provisioned workdir,
  scopes network/env, applies CPU/memory limits and the timeout, streams the prompt
  to the container's stdin. Driver-gated dependency so `local`-only builds add no
  Docker dep.
- **One agent per sandbox.** Simplifies the security model and cleanup; no shared
  sandbox state between runs.
- **Defense in depth.** The sandbox boundary is the last of three layers (hub
  authorization, runner grant enforcement, sandbox containment).

## Risks / Trade-offs

- **Docker availability/footprint.** Not every device has Docker; hence pluggable
  drivers and a `local` fallback. Container cold-start adds latency.
- **`local` driver risk.** Running untrusted agents under `local` is unsafe;
  mitigate by policy (grants should require an isolating driver for untrusted
  agents) and clear documentation.
- **Resource-limit portability.** Limits differ across drivers/OSes; define a
  common subset (CPU, memory, timeout) and allow driver-specific extensions.
- **Filesystem provisioning.** Getting inputs in and outputs out of an isolated
  sandbox needs a clear contract (mount the workdir; capture stdout/stderr).
