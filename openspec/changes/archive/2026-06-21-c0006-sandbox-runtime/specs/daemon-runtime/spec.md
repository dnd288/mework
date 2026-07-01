## MODIFIED Requirements

### Requirement: Safe, isolated execution

The system SHALL execute agents through the **sandbox runtime** (see the
`sandbox-runtime` capability) using the driver selected for the dispatch, rather
than via a bare host subprocess. The prompt MUST be fed over **stdin, never as a
shell argument**, each run MUST occur in its own sandbox (one agent per sandbox),
and each run MUST be bounded by a timeout (default 30 minutes). Isolation drivers
MUST confine the agent from the host; the `local` driver preserves today's
host-subprocess behavior for trusted use.

#### Scenario: Execution goes through a sandbox driver

- **WHEN** the runner runs a dispatched agent
- **THEN** it invokes the selected sandbox driver instead of calling the host process directly

#### Scenario: Prompt is not placed on the command line

- **WHEN** the runner runs a backend with ticket/agent-derived prompt content
- **THEN** the prompt is written to the sandbox process stdin and never appears in argv

#### Scenario: Runaway run is bounded

- **WHEN** a sandboxed run exceeds its timeout
- **THEN** the run is cancelled and the dispatch is reported as `failed`
