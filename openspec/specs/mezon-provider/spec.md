# Mezon Provider Specification

## Purpose

Define the Mezon provider adapter that registers with the provider gateway under the code `"mezon"` and implements the provider interface for chat-type providers. The adapter handles channel key extraction, event parsing, and write-back to Mezon channels. Owned by `libs/server/provider/mezon/`.

## Requirements

### Requirement: Provider adapter registration

The Mezon adapter SHALL register itself with the global provider registry under the code `"mezon"` without requiring a bot argument. The `RegisterAdapter()` function SHALL NOT accept a bot parameter. The adapter is registered standalone -- server-side write-back to Mezon is not supported (the worker handles outbound replies).

#### Scenario: Register adapter without bot

- **WHEN** the server starts and registers the Mezon adapter
- **THEN** `RegisterAdapter()` is called without a bot argument
- **THEN** the adapter is registered in the global provider registry under code `"mezon"`

#### Scenario: Look up Mezon adapter

- **WHEN** a component calls `provider.Get("mezon")`
- **THEN** it receives the Mezon adapter instance

### Requirement: Channel key extraction

The Mezon adapter SHALL implement `ChannelKey(rawPayload) -> (providerCode, resourceID)` that extracts the channel ID from a Mezon message event. The provider code SHALL be `"mezon"`. The resource ID SHALL be the `channel_id` from the message.

#### Scenario: Channel key from DM

- **WHEN** the adapter's `ChannelKey` is called with a DM message payload containing `channel_id = "dm_abc123"`
- **THEN** it returns `("mezon", "dm_abc123")`

#### Scenario: Channel key from group channel

- **WHEN** the adapter's `ChannelKey` is called with a group channel message payload containing `channel_id = "ch_xyz789"`
- **THEN** it returns `("mezon", "ch_xyz789")`

### Requirement: Event parsing from Mezon messages

The Mezon adapter SHALL implement `ParseEvent(payload)` that converts a Mezon channel message into a `CanonicalEvent`. The event ID SHALL be the message's unique ID. The event type SHALL be `"message.created"`. The actor SHALL be the message sender. The body SHALL be the message text content.

#### Scenario: Parse a channel message

- **WHEN** `ParseEvent` is called with a payload containing a channel message
- **THEN** it returns a `CanonicalEvent` with `EventType = "message.created"`, the sender's user ID as `Actor.ID`, and the message text as `Body`

### Requirement: Task detail for Mezon

The Mezon adapter SHALL implement `FetchTaskDetail(ctx, token, taskID)`. Since Mezon is a chat platform (not a task tracker), this method SHALL return empty strings for `Title` and `Description` -- there are no "tasks" in Mezon, only messages and channels.

#### Scenario: Fetch task detail returns empty

- **WHEN** `FetchTaskDetail` is called with any Mezon channel ID
- **THEN** it returns a `TaskDetail` with empty `Title` and `Description`

### Requirement: Webhook headers (no-op)

The Mezon adapter SHALL implement `WebhookHeaders()` returning empty header names. Since Mezon messages arrive via WebSocket (not webhooks), signature verification is not applicable. The method exists only to satisfy the `Provider` interface contract.

#### Scenario: Webhook headers are empty

- **WHEN** `WebhookHeaders()` is called on the Mezon adapter
- **THEN** it returns an empty `WebhookHeaderNames` struct

### Requirement: Container ID extraction (no-op)

The Mezon adapter SHALL implement `ExtractContainerID(body)` returning an empty string. Mezon does not have a "container" concept (it is chat, not a board-based tracker). The method exists only to satisfy the `Provider` interface contract.

#### Scenario: Container ID is empty

- **WHEN** `ExtractContainerID` is called with any payload
- **THEN** it returns `("", nil)`
