## MODIFIED Requirements

### Requirement: Safe, isolated execution

The system SHALL feed the prompt (and, for interactive sessions, **each turn**) to the
backend over **stdin, never as a shell argument** (ticket/turn content is
attacker-controllable), execute in an isolated per-job/per-session working directory,
and bound each run by a timeout. A **one-shot** run is bounded by a per-run timeout
(default 30 minutes). A **long-lived interactive** sandbox is instead bounded by its
**session lifecycle** — explicit close or idle reaping — rather than a single per-run
timeout, while individual turns MAY still carry a per-turn bound.

#### Scenario: Prompt is not placed on the command line

- **WHEN** the daemon runs a backend with ticket-derived prompt content
- **THEN** the prompt is written to the process stdin and never appears in argv

#### Scenario: Turn content is not placed on the command line

- **WHEN** the daemon delivers an interactive turn to a long-lived sandbox
- **THEN** the turn content is written to the process stdin and never appears in argv

#### Scenario: Runaway run is bounded

- **WHEN** a one-shot backend run exceeds its timeout
- **THEN** the run is cancelled and the job is acked `failed`

#### Scenario: Long-lived sandbox is bounded by its session

- **WHEN** an interactive session is closed or reaped for idleness
- **THEN** the daemon destroys the long-lived sandbox bounding its lifetime

## ADDED Requirements

### Requirement: Interactive sandbox session execution

The daemon SHALL be able to drive a **long-lived sandbox per session** in addition to
one-shot dispatch. It MUST start the sandbox once for the session, deliver each chat
turn to the running agent over stdin, support **cancel/interrupt** of an in-flight
turn without destroying the sandbox, and destroy the sandbox when the session closes
or is reaped. The **one-agent-per-sandbox** invariant MUST hold for the session.

#### Scenario: Sandbox persists across turns

- **WHEN** the daemon receives a second turn for an open session
- **THEN** it routes the turn to the same long-lived sandbox started for the first turn

#### Scenario: Cancel interrupts the turn but keeps the sandbox

- **WHEN** a cancel arrives for an in-flight turn
- **THEN** the daemon interrupts the turn and leaves the sandbox running for the next turn

### Requirement: Daemon streams session events

While driving an interactive session, the daemon SHALL **stream events** for each turn
(at least `token`, `message`, and one terminal `done`/`error`) to the session's topic
on the message bus, so subscribers can observe progress live.

#### Scenario: Events published per turn

- **WHEN** the daemon runs an interactive turn
- **THEN** it publishes `token`/`message` events and exactly one terminal `done` or `error` for that turn to the session topic
