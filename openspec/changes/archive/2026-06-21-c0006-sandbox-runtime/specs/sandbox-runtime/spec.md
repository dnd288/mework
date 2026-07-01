## ADDED Requirements

### Requirement: Sandbox driver interface

The system SHALL define a **Sandbox driver interface** with a lifecycle of
create → start → exec → stop → destroy, accepting a prompt over **stdin** and
returning captured output and an exit status. Execution paths MUST depend on this
interface, not on a concrete runtime, so drivers are swappable.

#### Scenario: Run an agent through the interface

- **WHEN** the runner executes a dispatched agent through the sandbox interface
- **THEN** the agent is created, started, fed its prompt over stdin, and its output and exit status are captured and returned

#### Scenario: Prompt is never placed on the command line

- **WHEN** an agent is executed with prompt content derived from untrusted input
- **THEN** the prompt is written to the sandbox process stdin and never appears in argv

### Requirement: Selectable engines

The system SHALL provide selectable sandbox engines behind one driver interface — at least
`local` (host subprocess, for trusted use) and `docker` (a container per agent) — and MUST
support remote/cloud engines (e.g. `cloudflare`) through the same interface. Each engine
lives in its own subpackage under `sandbox/engine/<name>` and isolates its third-party SDK;
the engine set MUST be extensible (including out-of-tree engines) without changing callers.

#### Scenario: Select the docker engine

- **WHEN** a dispatch (or configuration) selects the `docker` engine
- **THEN** the agent runs inside a container via the docker engine

#### Scenario: Select the local engine

- **WHEN** a dispatch (or configuration) selects the `local` engine
- **THEN** the agent runs as a host subprocess in an isolated working directory, preserving today's behavior

#### Scenario: Run on a remote engine through the same interface

- **WHEN** a dispatch selects a remote/cloud engine such as `cloudflare`
- **THEN** the run is provisioned and executed remotely behind the same `SandboxDriver`/`Sandbox` interface, with the runner code unchanged

#### Scenario: Add an out-of-tree engine without changing callers

- **WHEN** a new engine implementing the driver interface is shipped in an external module depending only on `shared`
- **THEN** existing execution callers use it through the same interface with no change

### Requirement: Capability-driven engine selection

Each engine SHALL advertise its capabilities (isolation level, network policy, persistence,
whether it runs remotely, and resource limits), and the engine for a run MUST be chosen from
those capabilities and the dispatch grant rather than hardcoded, so untrusted work is routed
to an isolating engine.

#### Scenario: Engine chosen by capability and policy

- **WHEN** a run is dispatched with a grant/policy requiring isolation
- **THEN** an engine whose advertised capabilities satisfy the policy is selected (e.g. an untrusted run is not placed on the `local` engine)

### Requirement: One agent per sandbox

The system SHALL run **at most one agent per sandbox** instance, and the sandbox
lifecycle MUST be bound to that single run — created for the run and destroyed
after it.

#### Scenario: Isolation between runs

- **WHEN** two agents are dispatched to the same runner
- **THEN** each runs in its own sandbox instance and neither shares the other's sandbox filesystem or process space

#### Scenario: Sandbox is torn down after the run

- **WHEN** an agent run reaches a terminal state
- **THEN** its sandbox is stopped and destroyed, releasing its resources

### Requirement: Isolation and resource limits

A sandbox driver that provides isolation SHALL confine the agent from the host
(filesystem, network, and environment scoped to what the run needs) and SHALL
support **resource limits** (e.g. CPU, memory, and a wall-clock timeout; default
timeout 30 minutes). The `local` driver MUST document that it does **not** provide
host isolation and is intended for trusted use.

#### Scenario: Isolated driver confines the agent

- **WHEN** an agent runs under the `docker` driver
- **THEN** it cannot read or write host paths outside those explicitly provisioned for the run

#### Scenario: Resource limit terminates a runaway agent

- **WHEN** an agent exceeds its configured wall-clock timeout or resource limit
- **THEN** the sandbox terminates the run and reports failure

#### Scenario: Concurrent sandboxes do not interfere

- **WHEN** two runners provision sandboxes for different runs at the same time
- **THEN** each sandbox is fully isolated and neither observes the other's filesystem, environment, or process space

### Requirement: Image-based agent engines

A container/cloud engine SHALL be able to pull and run a packaged **agent image** — an image
with the agent CLI (e.g. claude code, codex) preinstalled — provisioning the sandbox from
that image before the run.

#### Scenario: Pull and run an agent image

- **WHEN** an image-form agent version is dispatched to the `docker` engine
- **THEN** the engine pulls the referenced image, starts a container from it, and the sandbox becomes running for the run

### Requirement: Sandbox inspection

The runtime SHALL expose the lifecycle state of a provisioned sandbox (e.g. running,
stopped, destroyed, crashed) so operators and the runner can observe and manage in-flight
runs.

#### Scenario: Inspect a running sandbox

- **WHEN** the state of a provisioned, running sandbox is queried
- **THEN** the runtime reports its current lifecycle state

### Requirement: Crash handling

The runtime SHALL detect a sandbox that crashes mid-run, report the dispatch as `failed`,
release the sandbox's resources, and keep the runner available for the next dispatch. A
single sandbox crash MUST NOT take down the runner.

#### Scenario: A crashed sandbox is reported failed

- **WHEN** a running sandbox's process crashes mid-run
- **THEN** the dispatch is reported `failed` with a crash summary

#### Scenario: Resources are released after a crash

- **WHEN** the runtime reconciles a crashed sandbox
- **THEN** the sandbox is destroyed and its resources are released, leaving no leak

#### Scenario: The runner survives a sandbox crash

- **WHEN** one of a runner's sandboxes crashes
- **THEN** the runner reports that dispatch failed and remains online for subsequent dispatches

### Requirement: Agent backends

The sandbox SHALL run the dispatched agent through a selectable **agent backend** (e.g.
claude code, codex, opencode), feeding the prompt over **stdin, never argv**, and capturing
its output and exit status. Backend selection MUST be data-driven and detect an available
backend (the `local` engine detects a host CLI; an image engine uses the image's
preinstalled CLI); an unavailable backend MUST be reported rather than attempted.

#### Scenario: Run an agent through a backend

- **WHEN** a backend (e.g. claude code or codex) is selected for a run
- **THEN** the agent runs through that backend, the prompt is delivered on stdin, and its output and exit status are captured

#### Scenario: No installed backend is handled

- **WHEN** no agent backend is available for the run
- **THEN** the backend is reported unavailable and the run is not attempted on the host command line

### Requirement: Workspace bootstrap and mount

When a run carries a workspace (see the `workspaces` capability), the sandbox SHALL
materialize the workspace base code, run its lifecycle hooks (init/pre-run/post-run), and
mount the workspace before/around executing the agent, all within the run's grant scope and
with hook scripts fed over stdin.

#### Scenario: Bootstrap and mount before the agent runs

- **WHEN** a dispatched run carries a workspace with base code and hooks
- **THEN** the sandbox materializes the base code, runs the init/pre-run hooks, and mounts the workspace before executing the agent

#### Scenario: A failing bootstrap hook aborts the run

- **WHEN** a workspace `init`/`pre_run` hook exits non-zero during bootstrap
- **THEN** the run is aborted, reported `failed`, and the sandbox is torn down
