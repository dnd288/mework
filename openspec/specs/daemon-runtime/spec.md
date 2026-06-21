# Daemon Runtime Specification

## Purpose

Define the local agent daemon: a **stateless** long-poll worker that claims jobs
from `mework-server`, runs an AI coding CLI against the ticket in an isolated
workspace, and reports the result. It holds no provider credentials and persists
no job state of its own. Owned by `internal/daemon` and `internal/agentrun`.

## Requirements

### Requirement: Stateless poll worker

The daemon SHALL operate as an **enrolled SSE runner**, not a poll worker. After
one-time enrollment it MUST subscribe to its topics over SSE (per `message-bus`),
receive dispatches by push, and drive a **pull → run → report** loop, persisting
only its durable runner identity and in-flight bookkeeping. It MUST NOT poll a
claim endpoint on a fixed interval.

#### Scenario: No interval polling

- **WHEN** an enrolled runner is online and idle
- **THEN** it holds an open SSE subscription and issues no periodic claim/poll requests

#### Scenario: Driven by dispatch

- **WHEN** the hub dispatches work to the runner
- **THEN** the runner pulls the agent, runs it, and reports the result, acknowledging the dispatch on terminal handling

### Requirement: AI backend detection

The system SHALL detect an installed AI CLI backend (default discovery order
`claude` → `codex` → `opencode` via `PATH`) and execute the one selected by the
job's profile/backend.

#### Scenario: Fall back to the next backend

- **WHEN** the preferred backend is not found on `PATH`
- **THEN** the daemon selects the next available backend in discovery order

### Requirement: Safe, isolated execution

The system SHALL feed the prompt to the backend over **stdin, never as a shell
argument** (ticket content is attacker-controllable), execute in an isolated
per-job working directory, and bound each run with a timeout (default 30
minutes).

#### Scenario: Prompt is not placed on the command line

- **WHEN** the daemon runs a backend with ticket-derived prompt content
- **THEN** the prompt is written to the process stdin and never appears in argv

#### Scenario: Runaway run is bounded

- **WHEN** a backend run exceeds its timeout
- **THEN** the run is cancelled and the job is acked `failed`

### Requirement: Daemon lifecycle management

The system SHALL provide lifecycle controls — `daemon start` (background, or
`--foreground` in-process), `stop`, `status`, `restart`, and `logs` — with
per-profile pid, log, and work directories so multiple profiles can run isolated.

#### Scenario: Inspect a running daemon

- **WHEN** a user runs `mework daemon status`
- **THEN** the system reports whether the daemon is running and its health for the active profile

### Requirement: Spec-based enrollment

The daemon SHALL declare the agent specs it can execute during enrollment. Specs SHALL be sent as an array in the enrollment request body. The daemon SHALL determine its supported specs from the AI CLIs it has installed at startup time (e.g., if `claude` is found in PATH, the daemon includes `"claude-code"` in its specs).

#### Scenario: Enroll with detected specs

- **WHEN** the daemon starts and detects `claude` and `opencode` in PATH
- **THEN** it enrolls with `specs: ["claude-code", "opencode"]`

### Requirement: Channel subscription on sandbox provision

When the daemon receives a channel dispatch, the sandbox SHALL subscribe to the channel's topic on the message bus using the filter `channel.<provider>.<resource_id>.*`.

#### Scenario: Sandbox subscribes to its assigned channel

- **WHEN** a sandbox is provisioned for channel `"mello:TICKET-99"` on a runner
- **THEN** the sandbox opens a bus subscription with filter `channel.mello.TICKET-99.*`

### Requirement: Heartbeat with current specs

The daemon SHALL include its current specs in every heartbeat sent to the server, allowing the server to detect when a runner's capabilities change.

#### Scenario: Heartbeat updates specs

- **WHEN** the daemon heartbeats with `{"specs": ["claude-code"]}`
- **THEN** the server updates `runtimes.specs` for that runner
