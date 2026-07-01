## ADDED Requirements

### Requirement: Open-session dispatch drives the interactive session

The daemon SHALL treat a dispatch identified by a non-empty session id as an
**open-session dispatch** and drive a **long-lived interactive session** for that id rather
than running a one-shot agent. It MUST verify the dispatch grant (requiring spawn
permission), authorize against the **owner and tenant** carried on the dispatch, resolve the
agent definition, and start the sandbox **exactly once** for the session. The daemon SHALL
then **consume chat turns from the session's input topic** and route each turn to the
running session **serially**, and SHALL **deliver the session's per-turn events to the
server** for relay to subscribers.
A **duplicate** open-session dispatch for an already-open session id MUST NOT start a second
sandbox. Closing or cancelling the session MUST follow the interactive-session lifecycle
(cancel interrupts an in-flight turn; close destroys the sandbox).

#### Scenario: Open-session dispatch starts one long-lived sandbox

- **WHEN** the daemon receives a dispatch carrying a session id, owner, tenant, and a
  pull+spawn grant
- **THEN** it authorizes the caller, resolves the definition, and opens the session's
  sandbox once

#### Scenario: Turns from the input topic route to the session

- **WHEN** chat turns arrive on the session's input topic for an open session
- **THEN** the daemon delivers each turn to the same long-lived sandbox, one after another

#### Scenario: Duplicate dispatch is idempotent

- **WHEN** the daemon receives a second open-session dispatch for a session it already has
  open (e.g. on stream resume/redelivery)
- **THEN** it does not start a second sandbox for that session

#### Scenario: Per-turn events reach the server

- **WHEN** the daemon runs an interactive turn for a server-routed session
- **THEN** it delivers the turn's `token`/`message` events and one terminal `done`/`error`
  to the server so subscribers can observe them
