# Auth and Secrets Specification

## Purpose

Define the authentication and credential-protection model for `mework-server`:
two distinct token types guarding two route classes, AES-256-GCM sealing of
provider credentials at rest, and HMAC-based runtime-token lookup. Owned by
`internal/server/{auth,middleware,token,secret}`.
## Requirements
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

### Requirement: Runtime token generation and lookup

The system SHALL generate runtime tokens with a recognizable prefix and
sufficient entropy (256-bit), return the plaintext token to the caller exactly
once at registration, and store only an HMAC-SHA256 lookup hash (keyed by
`SERVER_KEY`) so the raw token is never persisted.

#### Scenario: Token shown once

- **WHEN** a runtime is registered
- **THEN** the plaintext `rt_token` is returned once and only its HMAC lookup hash is stored

#### Scenario: Authenticate by lookup hash

- **WHEN** a daemon presents its `rt_token`
- **THEN** the server hashes it with `SERVER_KEY` and matches the stored lookup hash to identify the runtime

### Requirement: Credential sealing at rest

The system SHALL seal provider credentials with AES-256-GCM using
`MEWORK_SECRET_KEY` before storing them, and unseal them only server-side at the
moment they are needed for write-back.

#### Scenario: Stored credential is encrypted

- **WHEN** a provider connection credential is saved
- **THEN** it is stored as AES-256-GCM ciphertext, not plaintext

#### Scenario: Unseal only for write-back

- **WHEN** a write-back needs the provider credential
- **THEN** the server unseals it in memory for the call and does not expose it elsewhere

### Requirement: Required server secrets

The system SHALL require `DATABASE_URL`, `SERVER_KEY`, and `MEWORK_SECRET_KEY` at startup and
MUST fail fast if any is missing. The system SHALL additionally enforce a **minimum length**
for `SERVER_KEY` and `MEWORK_SECRET_KEY` and MUST fail fast, naming the offending variable,
when either is shorter than the minimum — so a trivially-weak key is rejected rather than
silently accepted and stretched.

#### Scenario: Missing secret aborts startup

- **WHEN** the server starts without `MEWORK_SECRET_KEY`
- **THEN** it refuses to start rather than run without credential sealing

#### Scenario: Weak key aborts startup

- **WHEN** the server starts with a `SERVER_KEY` or `MEWORK_SECRET_KEY` shorter than the
  minimum length
- **THEN** it refuses to start and the error names the offending variable and the minimum

