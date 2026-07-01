# Channel Routing Specification

## Purpose

Define how `mework-server` routes incoming events from any provider to a per-resource sandbox session through a channel-addressed routing layer. The channel router decouples event sources (webhook handlers, API adapters) from sandbox execution, enabling provider-agnostic, resource-scoped event delivery. Owned by `libs/server/channel/`.

## ADDED Requirements

### Requirement: Channel key computation

The channel router SHALL compute a deterministic channel key from every incoming event using the format `(provider_code, external_resource_id)`. The provider code is the registered adapter code (e.g. `mello`, `github`). The external resource ID is the provider's native identifier for the ticket, issue, PR, or equivalent work item. The key SHALL be a colon-joined string: `"mello:ticket-abc123"`.

#### Scenario: Channel key from Mello event

- **WHEN** a webhook event arrives for provider `mello` with `ticket_id = "TICKET-99"`
- **THEN** the channel key is `"mello:TICKET-99"`

#### Scenario: Channel key from GitHub event

- **WHEN** a webhook event arrives for provider `github` with `issue_id = "42"`
- **THEN** the channel key is `"github:42"`

### Requirement: Route event to active session

The channel router SHALL look up the channel key in the channel registry. If an active session is bound, the router SHALL publish the event to the channel's bus topic and the sandbox receives it through its subscription. The lookup SHALL be served from an in-memory cache first, falling back to the DB.

#### Scenario: Event routed to existing session

- **WHEN** a channel key `"mello:TICKET-99"` has an active session bound, and a new event arrives for that key
- **THEN** the event is published to topic `channel.mello.TICKET-99.dispatch` and the bound sandbox receives it

#### Scenario: No active session triggers auto-provision

- **WHEN** a channel key has no active session bound
- **THEN** the router calls the auto-provisioner to create a session, bind the channel, and deliver the event

### Requirement: Provider-agnostic routing

The channel router SHALL be provider-agnostic: it MUST NOT reference provider-specific fields (Mello `board_id`, GitHub `issue number`, Jira `project key`) directly. All provider-specific extraction SHALL happen in the adapter's `ChannelKey(event)` method, which returns the normalized `(provider_code, resource_id)` pair.

#### Scenario: Same routing for any provider

- **WHEN** events arrive from `mello`, `github`, and `jira` adapters
- **THEN** the channel router treats them identically, routing by channel key without inspecting provider-specific fields

### Requirement: Event serialization per channel

The channel router SHALL deliver events for the same channel key in publication order. Concurrent publishes to the same channel MUST be serialized so the sandbox processes them one at a time. Different channels MAY be processed concurrently.

#### Scenario: Sequential delivery for one resource

- **WHEN** two events are published for channel `"mello:TICKET-99"` concurrently
- **THEN** they are delivered to the bound sandbox one after the other, in publish order

#### Scenario: Concurrent delivery across resources

- **WHEN** events are published for channels `"mello:TICKET-99"` and `"github:42"` concurrently
- **THEN** both sandboxes receive their events simultaneously

### Requirement: Channel lifecycle observability

The channel router SHALL expose a status endpoint at `GET /api/v1/channels` listing all active channels with their bound session ID, provider code, resource ID, runner ID, and current status (`active`, `draining`, `closed`). The endpoint SHALL be PAT-authenticated.

#### Scenario: List active channels

- **WHEN** an authenticated user requests `GET /api/v1/channels`
- **THEN** the response includes all active channel sessions with their metadata

#### Scenario: Channel transitions to draining

- **WHEN** the sandbox finishes processing the final event for a resource
- **THEN** the channel status transitions to `draining` and accepts no new events
- **AFTER** all in-flight events complete, the channel transitions to `closed`
