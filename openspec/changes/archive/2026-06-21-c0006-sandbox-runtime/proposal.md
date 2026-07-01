## Why

A core promise of the redesign is running operator-dispatched agents on a device
**without operating on the client's machine** — i.e. the work must be **isolated**
from the host. Today there is no isolation: `agentrun` runs the AI CLI with a bare
`exec.CommandContext` in a per-job host directory (`sandbox/engine/local/runner.go:37`),
as the same OS user, sharing the host filesystem, network, and environment. That is
acceptable for a trusted local CLI but unsafe for pulling and running arbitrary
permitted agents dispatched from a hub.

This change introduces a **pluggable sandbox runtime**: a driver interface with
multiple drivers (`local`, `docker`, and others) so each dispatched agent runs in
an isolated runtime, one agent per sandbox.

## What Changes

- A new **sandbox-runtime** capability: a Sandbox driver interface
  (create/start/exec/stop/destroy, resource limits, stdin prompt, captured output)
  with selectable drivers; **one agent per sandbox**.
- Drivers: `local` (current host-subprocess behavior, default for trusted use) and
  `docker` (a container per agent); extensible to other isolation backends.
- Agent execution moves from the bare `exec.CommandContext` seam to the selected
  driver; the prompt is still fed over **stdin, never argv**.

## Capabilities

### New Capabilities
- `sandbox-runtime`: pluggable, isolated, one-agent-per-sandbox execution with a
  driver interface and at least `local` and `docker` drivers.

### Modified Capabilities
- `daemon-runtime`: agent execution routes through the selected sandbox driver
  instead of a bare host subprocess.

## Impact

- **Sequenced after `c0002-repo-restructure`** (and `c0005-agent-runner`): execution moves to
  `sandbox/runtime` (was `sandbox/engine/local`), with one subpackage per
  driver (`sandbox/engine/local`, `sandbox/engine/docker`, …).
- Affected code: `sandbox/engine/*` (driver interface + drivers), the
  former `runner.go` exec seam, the runner's execution path.
- New optional dependency: a Docker client for the `docker` driver (driver-gated;
  `local` keeps zero new deps).
- Consumed by `agent-runner` (the run step of pull → run → report).
