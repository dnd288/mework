## MODIFIED Requirements

### Requirement: Idempotent enqueue

The system SHALL de-duplicate inbound provider events using a unique key on
`(provider_code, external_event_id)`, and SHALL **publish** at most one message to
the target topic per unique event. Redelivered webhooks MUST NOT produce duplicate
published messages. (Previously this requirement guaranteed at-most-one *enqueued
job*; under the message bus the same guarantee applies to the *published
message*.)

#### Scenario: Duplicate webhook delivery

- **WHEN** the same provider event is delivered more than once
- **THEN** at most one message is published to the topic for that `(provider_code, external_event_id)`

#### Scenario: Distinct events publish distinct messages

- **WHEN** two different provider events arrive with different `external_event_id` values
- **THEN** each results in its own published message on the topic
