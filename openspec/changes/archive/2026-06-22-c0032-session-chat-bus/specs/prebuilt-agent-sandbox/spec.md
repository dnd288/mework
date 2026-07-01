## ADDED Requirements

### Requirement: Server-routed chat transport

The server SHALL provide HTTP transport for an interactive chat over a session, acting as a
**thin bus relay that holds no conversation state**. An authenticated, owning caller SHALL
be able to **submit a chat turn** for a session and **stream the session's events** back.
The runner SHALL be able to **deliver per-turn events** to the server for relay. Submitting
a turn publishes it to the session's input topic; streaming subscribes to the session's
control topic; runner-delivered events are republished on the control topic. Conversation
state and the sandbox remain on the runner.

#### Scenario: Submit a chat turn

- **WHEN** an owning caller issues `POST /api/v1/sessions/{id}/messages` with a chat message
- **THEN** the server publishes the turn to the session's input topic for the runner and
  accepts the request, storing no conversation state

#### Scenario: Stream session events

- **WHEN** an owning caller issues `GET /api/v1/sessions/{id}/stream`
- **THEN** the server streams the session's `token`/`message`/`done`/`error` events as they
  arrive on the session's control topic

#### Scenario: Non-owner cannot submit or stream

- **WHEN** a caller who does not own the session attempts to submit a turn or stream its
  events
- **THEN** the server denies the request

#### Scenario: Runner delivers events for relay

- **WHEN** the runner posts a session event to `POST /api/v1/runners/sessions/{id}/events`
  with its runtime credential
- **THEN** the server republishes it on the session's control topic so subscribers receive
  it
