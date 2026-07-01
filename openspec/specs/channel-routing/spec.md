# Channel Routing Specification

## Purpose

Define how `mework-server` routes incoming events from any provider to a per-resource sandbox session through a channel-addressed routing layer. The channel router decouples event sources from sandbox execution, enabling provider-agnostic, resource-scoped event delivery. Owned by `libs/server/channel/`.
## Requirements
### Requirement: Channel key computation

The channel router SHALL compute a deterministic channel key from every incoming event using the format `(provider_code, external_resource_id)`. The key SHALL be a colon-joined string: `"mello:ticket-abc123"`.

#### Scenario: Channel key from Mello event

- **WHEN** a webhook event arrives for provider `mello` with `ticket_id = "TICKET-99"`
- **THEN** the channel key is `"mello:TICKET-99"`

#### Scenario: Channel key from Mezon message

- **WHEN** a chat message arrives for provider `mezon` with `channel_id = "ch_abc123"`
- **THEN** the channel key is `"mezon:ch_abc123"`

### Requirement: Route event to active session

The channel router SHALL look up the channel key in the channel registry. If an active session is bound, the router SHALL publish the event to the channel's bus topic.

#### Scenario: Event routed to existing session

- **WHEN** a channel key `"mello:TICKET-99"` has an active session, and a new event arrives
- **THEN** the event is published to topic `channel.mello.TICKET-99.dispatch` and the bound sandbox receives it

#### Scenario: No active session triggers auto-provision

- **WHEN** a channel key has no active session
- **THEN** the router calls the auto-provisioner to create a session, bind the channel, and deliver the event

#### Scenario: Mezon message routed to existing session

- **WHEN** a channel key `"mezon:ch_abc"` has an active session, and a new chat message arrives
- **THEN** the message is published to topic `channel.mezon.ch_abc.dispatch` and the bound sandbox receives it

### Requirement: Provider-agnostic routing

The channel router SHALL be provider-agnostic. All provider-specific extraction SHALL happen in the adapter's method that returns the normalized `(provider_code, resource_id)` pair.

#### Scenario: Same routing for any provider

- **WHEN** events arrive from `mello`, `mezon`, `github`, and `jira` adapters
- **THEN** the channel router treats them identically, routing by channel key

### Requirement: Channel lifecycle observability

The channel router SHALL expose a status endpoint at `GET /api/v1/channels` listing all active channels with their bound session ID, provider code, resource ID, runner ID, and current status.

#### Scenario: List active channels

- **WHEN** an authenticated user requests `GET /api/v1/channels`
- **THEN** the response includes all active channel sessions with their metadata

### Requirement: Channel routing is opt-in and disabled by default

Channel routing (per-resource session auto-provisioning) SHALL be **disabled by default** and
enabled only by explicit configuration. When disabled, verified webhook events SHALL be handled
by the legacy pipeline (enqueue → claim → write-back) with no dependence on the channel
auto-provisioner. The feature flag SHALL be configurable via environment so an operator can opt
in (e.g. for end-to-end testing) without a code change.

#### Scenario: Default deployment uses the legacy pipeline

- **WHEN** the server starts without channel routing explicitly enabled
- **THEN** channel routing is off and verified webhooks are handled by the legacy
  enqueue/claim/write-back pipeline

#### Scenario: Channel routing can be enabled by configuration

- **WHEN** the channel-routing environment flag is set to enabled
- **THEN** the server activates channel routing for that deployment

### Requirement: Real-time chat channel routing

The channel router SHALL support routing from real-time chat providers (Mezon) where messages arrive over a persistent WebSocket connection rather than HTTP webhooks. The router SHALL be invoked by the bot's message dispatch callback, not only by the webhook handler. The routing behavior (key computation → session lookup → publish or auto-provision) SHALL be identical regardless of whether the event source is a webhook or a WebSocket.

#### Scenario: Route Mezon message from bot callback

- **WHEN** the Mezon bot receives a channel message and calls `router.Route()` with `providerCode = "mezon"`, `resourceID = "ch_abc"`, and the message payload
- **THEN** the router computes key `"mezon:ch_abc"` and either delivers to the bound session or auto-provisions

### Requirement: Mezon message sources bypass webhook handler

Messages from Mezon WebSocket SHALL NOT enter through the `POST /webhooks/{provider}` endpoint. They SHALL enter the channel router directly from the bot's message dispatch callback. The router SHALL accept message payloads from any caller, not only from the webhook handler.

#### Scenario: Direct route from bot

- **WHEN** the Mezon bot's dispatch callback calls `Route()` directly
- **THEN** the event is routed identically to a webhook-sourced event of the same channel key

