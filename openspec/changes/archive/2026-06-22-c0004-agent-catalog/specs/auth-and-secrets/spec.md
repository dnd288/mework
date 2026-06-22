## MODIFIED Requirements

### Requirement: Two-token authentication

The system SHALL guard management routes with a Mello personal access token (PAT)
authenticator and guard runner/agent transport routes (SSE subscribe, ack, pull)
with a runner identity credential. In addition, every dispatched unit of work
SHALL carry a **scoped permission grant** describing the operations it is permitted
to perform; authentication establishes *who* the caller is, while the grant
establishes *what this run may do*. `/webhooks/{provider}` remains signature-verified
and `/healthz` remains open.

#### Scenario: PAT required for management routes

- **WHEN** a request hits a management route without a valid PAT
- **THEN** the system rejects it as unauthorized

#### Scenario: Runner credential required for transport routes

- **WHEN** a runner calls a subscribe/ack/pull route without a valid runner credential
- **THEN** the system rejects it as unauthorized

#### Scenario: Grant scopes the operation, not just identity

- **WHEN** an authenticated runner attempts an operation outside the grant attached to its current dispatch
- **THEN** the operation is denied even though the caller is authenticated
