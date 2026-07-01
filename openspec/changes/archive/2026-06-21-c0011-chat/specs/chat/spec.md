## ADDED Requirements

### Requirement: Send a message and stream the response

The system SHALL let an operator open a `Conversation` against an attached session
bound to a running agent and `Send` a `ChatMessage`. The assistant's reply MUST be
streamed back on the conversation's `Stream` as `ChatEvent`s (`Kind` ∈ `token` |
`message`), and each assistant turn MUST terminate with exactly one terminal event
(`done` on success, `error` on failure). The assistant response MUST be delivered
over the message bus rather than a separate transport.

#### Scenario: Assistant reply streams back

- **WHEN** the operator sends a user message on an attached session's conversation
- **THEN** the send succeeds and the assistant's reply arrives on the stream as `token`/`message` events ending in a single `done` event

#### Scenario: Failure surfaces as an error event

- **WHEN** an assistant turn fails or is refused
- **THEN** the turn terminates with a single `error` event rather than `done`

### Requirement: Multi-turn history is preserved

The system SHALL preserve conversation `History` across turns. `History` MUST
return every `ChatMessage` of the conversation in the order it was added, so a
follow-up message can refer to prior turns within the same session.

#### Scenario: History accumulates across turns

- **WHEN** the operator sends one message, then sends a follow-up referring to it
- **THEN** `History` returns both turns in the order they were sent

### Requirement: Cancel an in-flight turn

The system SHALL let the operator `Cancel` the in-flight assistant turn. Cancel
MUST stop the turn's stream promptly and MUST leave the session usable, so a
subsequent `Send` starts a fresh turn.

#### Scenario: Mid-turn cancel keeps the session usable

- **WHEN** the operator cancels while an assistant turn is streaming
- **THEN** the stream stops promptly and the next `Send` on the same session starts a new turn successfully

### Requirement: Concurrent conversations are isolated per session

The system SHALL isolate conversations by session. Each conversation's `Stream`
MUST carry only that conversation's events; a message or assistant turn in one
session MUST NOT appear on another session's stream, even when conversations run
concurrently.

#### Scenario: No cross-talk between sessions

- **WHEN** two sessions each send a message at the same time
- **THEN** each conversation streams only its own assistant response and neither observes the other's events

### Requirement: A system prompt steers and leads history

The system SHALL accept a `ChatMessage` with `Role` `system`. A system message
MUST apply to the turns of the conversation, and when present it MUST lead the
recorded `History` as the first message.

#### Scenario: System message leads history

- **WHEN** the conversation is opened with a system message and the operator then sends a user message
- **THEN** `History` records the system message first and it applies to the turn

### Requirement: Backpressure does not stall other sessions

The system SHALL apply backpressure per conversation. When a chat client drains its
`Stream` slower than the assistant produces events, the buffering or blocking MUST
be contained to that conversation and MUST NOT stall the agent or other sessions'
streams.

#### Scenario: Slow reader is isolated

- **WHEN** one conversation's client reads its stream slowly while another session is also active
- **THEN** backpressure is applied only to the slow conversation and the other session's stream keeps flowing
