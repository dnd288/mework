# Message Bus Specification

## Purpose

Define the message bus transport layer that connects server components (webhook ingestion, orchestrator, operators) to subscribing clients (agents, sandboxes, runners) over a Server-Sent Events stream with topic-based publish/subscribe, resumable delivery, delivery acknowledgement, and a pluggable broker backend interface. Owned by `internal/server/messagebus`.
## Requirements
### Requirement: Topic-based publish

The server SHALL publish messages to named **topics**. Producers (channel router, webhook ingestion, orchestrator, operators) MUST publish to a topic without knowing which clients are subscribed, and the set of subscribers MAY change at any time. The topic namespace SHALL include `channel.<provider>.<resource_id>.<event_type>` for per-resource event delivery alongside the existing `runner.<id>.dispatch` and `session.<id>.control` patterns.

#### Scenario: Publish to a topic with subscribers

- **WHEN** a producer publishes a message to topic `runner.<id>.dispatch`
- **THEN** the message is delivered to every client currently subscribed to that topic

#### Scenario: Publish to a channel topic with subscribers

- **WHEN** the channel router publishes a message to topic `channel.mello.TICKET-99.dispatch` and a sandbox is subscribed to `channel.mello.TICKET-99.*`
- **THEN** the message is delivered to that sandbox

#### Scenario: Publish to a topic with no subscribers

- **WHEN** a producer publishes to a topic that has no active subscribers
- **THEN** the publish succeeds and the message is retained for delivery to future subscribers, not dropped as a transport error

### Requirement: Channel-scoped subscription filters

A subscriber SHALL be able to subscribe to a channel topic using the existing wildcard filter pattern. The filter `channel.mello.TICKET-99.*` SHALL match `channel.mello.TICKET-99.dispatch`, `channel.mello.TICKET-99.control`, and `channel.mello.TICKET-99.status`. This reuses the existing `**` / `*` segment matching.

#### Scenario: Sandbox subscribes to its channel

- **WHEN** a sandbox is provisioned for channel `mello:TICKET-99`
- **THEN** it subscribes with filter `channel.mello.TICKET-99.*`
- **AND** receives only events for that channel, not other channels

#### Scenario: Channel isolation

- **WHEN** messages are published to `channel.mello.TICKET-99.dispatch` and `channel.github.42.dispatch`
- **THEN** the sandbox subscribed to `channel.mello.TICKET-99.*` receives only the first, not the second

### Requirement: SSE subscription contract

Clients SHALL subscribe to one or more topics over a single **Server-Sent Events** stream (`Content-Type: text/event-stream`). The server MUST push messages as SSE events as they are published; the client MUST NOT need to poll. Each event MUST carry a monotonic `id` so the stream is resumable.

#### Scenario: Receive a pushed message

- **WHEN** a client holds an open SSE subscription to topic `T` and a message is published to `T`
- **THEN** the server writes an SSE event to that client's stream without the client issuing a new request

#### Scenario: Subscribe to multiple topics on one stream

- **WHEN** a client opens an SSE subscription requesting topics `A` and `B`
- **THEN** messages from both topics arrive on the same stream, each tagged with its topic

### Requirement: Resumable delivery

The transport SHALL support resumption: when a reconnecting client presents the last event id it processed (the `Last-Event-ID` header), the server MUST resume delivery by sending only messages newer than that id that are still within the backing store's retention window — not from the beginning, and not only from "now".

#### Scenario: Resume after a dropped connection

- **WHEN** a client reconnects with `Last-Event-ID` set to the last event it processed
- **THEN** the server delivers messages newer than that id and does not redeliver already-processed messages

### Requirement: Delivery acknowledgement

A subscribed client SHALL acknowledge terminal handling of a message out-of-band over POST (not over the SSE stream, which is server→client only). Until a message is acknowledged or its lease expires, the server MUST be able to redeliver it to preserve at-least-once handling.

#### Scenario: Ack marks a message handled

- **WHEN** a client finishes handling a delivered message and POSTs an acknowledgement for it
- **THEN** the server marks the message handled and does not redeliver it

#### Scenario: Unacked message is redeliverable

- **WHEN** a delivered message's lease expires without an acknowledgement
- **THEN** the message remains unacknowledged and becomes eligible for redelivery — the server does not mark it handled or drop it

### Requirement: Pluggable broker backend

The server SHALL implement the bus behind a broker-backend interface so the durability/scale substrate can be swapped **without changing the client-facing SSE contract**. The default backend is Postgres `LISTEN/NOTIFY`. (Candidate alternative backends are evaluated in `design.md`, not fixed by this requirement.)

#### Scenario: Swap the backend without breaking clients

- **WHEN** the deployment switches the broker backend to a different implementation of the broker-backend interface
- **THEN** subscribed clients observe no change to the SSE subscription contract or event format

### Requirement: Smart filtered subscription

A subscription SHALL support a filter — exact topics and hierarchical wildcards (e.g. `session.<id>.*`) — so a subscriber receives only the events it asks for and the broker need not consider it for non-matching topics.

#### Scenario: Filter delivers only matching events

- **WHEN** a subscriber opens a subscription with the filter `session.s1.*` and the server publishes to both `session.s1.ctrl` and `session.s2.ctrl`
- **THEN** only the `session.s1.ctrl` event is delivered to that subscriber

### Requirement: Lazy delivery

The broker SHALL NOT materialize or buffer non-matching messages for a subscriber. Work for a subscriber MUST be proportional to the messages on the topics it is entitled to and filtered for, not to total system traffic.

#### Scenario: Non-matching traffic is not materialized for a subscriber

- **WHEN** a subscriber is filtered to `session.s1.*` and a high volume of `session.s2.*` messages is published
- **THEN** the broker does not enqueue or buffer those non-matching messages for that subscriber

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

### Requirement: Bounded per-subscriber backpressure

The bus SHALL absorb a slow subscriber without blocking publishers or other subscribers, applying bounded per-subscriber buffering or lease-based redelivery so one slow consumer does not stall the system.

#### Scenario: A slow subscriber does not stall the bus

- **WHEN** the hub publishes faster than one subscriber drains its stream
- **THEN** publishing is not blocked and other subscribers continue to receive their messages

#### Scenario: Per-topic ordering under concurrent publish

- **WHEN** messages are published concurrently to a single topic
- **THEN** each subscriber receives that topic's messages in per-topic order (best-effort; no global cross-topic ordering is guaranteed)

