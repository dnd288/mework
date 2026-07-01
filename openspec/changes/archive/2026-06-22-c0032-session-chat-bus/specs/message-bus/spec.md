## MODIFIED Requirements

### Requirement: Session control channel and push to sandbox

The bus SHALL provide **two per-session topics with a single direction each**, isolated per
session:

- `session.<id>.input` — **hub → runner**: carries chat turns and control messages (e.g.
  cancel, close) down to the running sandbox/agent, which subscribes to it.
- `session.<id>.control` — **runner → hub**: carries the session's outgoing events
  (`token`, `message`, `done`, `error`) up from the runner; the hub subscribes and relays
  them to session subscribers.

A subscriber on one session's topics MUST NOT receive another session's messages, and a
turn published to a session's input topic MUST NOT be delivered on that session's control
topic (no cross-direction leakage).

#### Scenario: Push a turn to a running sandbox

- **WHEN** the hub publishes a chat turn to `session.s1.input` while session `s1` has a
  running sandbox subscribed to it
- **THEN** the running agent receives the turn over its input channel

#### Scenario: Outgoing events flow on the control topic

- **WHEN** the runner publishes a `token`/`message`/`done` event for session `s1`
- **THEN** it is published on `session.s1.control` and the hub relays it to that session's
  subscribers

#### Scenario: Input and control are isolated and single-direction

- **WHEN** a turn is published to `session.s1.input`
- **THEN** a subscriber on `session.s1.control` receives nothing, and a subscriber on
  `session.s2.input` receives nothing (no cross-session, no cross-direction leakage)
