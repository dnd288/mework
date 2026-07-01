# Message Bus Specification — Delta

## MODIFIED Requirements

### Requirement: Topic-based publish

The server SHALL publish messages to named **topics**. Producers (channel router, webhook ingestion, orchestrator, operators) MUST publish to a topic without knowing which clients are subscribed, and the set of subscribers MAY change at any time. The topic namespace SHALL include `channel.<provider>.<resource_id>.<event_type>` for per-resource event delivery alongside the existing `runner.<id>.dispatch` and `session.<id>.control` patterns.

#### Scenario: Publish to a channel topic with subscribers

- **WHEN** the channel router publishes a message to topic `channel.mello.TICKET-99.dispatch` and a sandbox is subscribed to `channel.mello.TICKET-99.*`
- **THEN** the message is delivered to that sandbox

#### Scenario: Publish to a topic with no subscribers

- **WHEN** a producer publishes to a topic that has no active subscribers
- **THEN** the publish succeeds and the message is retained for delivery to future subscribers, not dropped as a transport error

### Requirement: Channel-scoped subscription filters

A subscriber SHALL be able to subscribe to a channel topic using the existing wildcard filter pattern. The filter `channel.mello.TICKET-99.*` SHALL match `channel.mello.TICKET-99.dispatch`, `channel.mello.TICKET-99.control`, and `channel.mello.TICKET-99.status`. This reuses the existing `**` / `*` segment matching in `MatchTopic` — no new filter syntax is required.

#### Scenario: Sandbox subscribes to its channel

- **WHEN** a sandbox is provisioned for channel `mello:TICKET-99`
- **THEN** it subscribes with filter `channel.mello.TICKET-99.*`
- **AND** receives only events for that channel, not other channels

#### Scenario: Channel isolation

- **WHEN** messages are published to `channel.mello.TICKET-99.dispatch` and `channel.github.42.dispatch`
- **THEN** the sandbox subscribed to `channel.mello.TICKET-99.*` receives only the first, not the second

## REMOVED Requirements

### Requirement: Existing runner.profile.dispatch as primary path

**Reason**: The `runner.<profile>.dispatch` topic is superseded by channel-scoped topics as the primary routing mechanism for external events. The old topic pattern is retained for backward compatibility but is no longer the recommended or default path.
**Migration**: New deployments use channel topics. Existing deployments may continue using `runner.<profile>.dispatch` until all runners are migrated to spec-based enrollment.
