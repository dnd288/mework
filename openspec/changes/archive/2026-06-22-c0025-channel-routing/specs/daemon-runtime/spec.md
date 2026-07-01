# Daemon Runtime Specification — Delta

## ADDED Requirements

### Requirement: Spec-based enrollment

The daemon SHALL declare the agent specs it can execute during enrollment. Specs SHALL be sent as an array in the enrollment request body: `{"code": "...", "label": "...", "specs": ["claude-code", "codex"]}`. The daemon SHALL determine its supported specs from the AI CLIs it has installed at startup time (e.g., if `claude` is found in PATH, the daemon includes `"claude-code"` in its specs).

#### Scenario: Enroll with detected specs

- **WHEN** the daemon starts and detects `claude` and `opencode` in PATH
- **THEN** it enrolls with `specs: ["claude-code", "opencode"]`

#### Scenario: Enroll without supported backend

- **WHEN** the daemon starts and detects no supported AI CLI in PATH
- **THEN** it enrolls with `specs: []` and is not eligible for any channel dispatch

### Requirement: Channel subscription on sandbox provision

When the daemon receives a channel dispatch, the sandbox SHALL subscribe to the channel's topic on the message bus using the filter `channel.<provider>.<resource_id>.*`. This SHALL be in addition to (or replacing) the existing SSE subscription to `runner.<profile>.dispatch`.

#### Scenario: Sandbox subscribes to its assigned channel

- **WHEN** a sandbox is provisioned for channel `"mello:TICKET-99"` on a runner
- **THEN** the sandbox opens a bus subscription with filter `channel.mello.TICKET-99.*`

#### Scenario: Sandbox receives only its channel events

- **WHEN** events are published to `channel.mello.TICKET-99.dispatch` and `channel.mello.TICKET-98.dispatch`
- **THEN** the sandbox for `TICKET-99` receives only the first

### Requirement: Heartbeat with current specs

The daemon SHALL include its current specs in every heartbeat sent to the server, allowing the server to detect when a runner's capabilities change (e.g., a new AI CLI was installed or uninstalled).

#### Scenario: Heartbeat updates specs

- **WHEN** the daemon heartbeats with `{"specs": ["claude-code"]}`
- **THEN** the server updates `runtimes.specs` for that runner
