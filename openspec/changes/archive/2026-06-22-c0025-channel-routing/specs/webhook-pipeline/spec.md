# Webhook Pipeline Specification — Delta

## ADDED Requirements

### Requirement: Publish to channel router

After trigger parsing and profile resolution, the webhook handler SHALL call the channel router to deliver the event to the appropriate channel, rather than (or in addition to) publishing directly to `runner.<profile>.dispatch`. The profile name resolves to a spec (via `backend_hint`), which the channel router uses for runner selection.

#### Scenario: Webhook triggers channel routing

- **WHEN** a valid webhook arrives with trigger `@mework dev review fix the bug`
- **THEN** the handler resolves profile `dev`, calls the channel router with key `"mello:TICKET-99"` and spec derived from the profile's `backend_hint`
- **AND** the channel router routes the event to the bound session or auto-provisions one

### Requirement: Adapter exposes normalized channel tuple

Each provider adapter SHALL expose a method that returns the normalized `(provider_code, resource_id)` pair from a raw event payload, enabling the channel router to compute the channel key without provider-specific knowledge. This method SHALL be called by the webhook handler during event processing.

#### Scenario: Mello adapter returns channel tuple

- **WHEN** the Mello adapter parses a webhook with `ticket_id = "TICKET-99"`
- **THEN** it returns `("mello", "TICKET-99")`

#### Scenario: GitHub adapter returns channel tuple

- **WHEN** a future GitHub adapter parses a webhook with `issue.number = 42`
- **THEN** it returns `("github", "42")`

## RENAMED Requirements

### Requirement: Webhook ingestion endpoint → Webhook ingestion and channel routing

**FROM**: Webhook ingestion endpoint
**TO**: Webhook ingestion and channel routing
**Reason**: The endpoint now both ingests the webhook and routes the event through the channel router.
