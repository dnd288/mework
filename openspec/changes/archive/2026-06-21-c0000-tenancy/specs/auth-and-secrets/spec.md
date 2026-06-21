## MODIFIED Requirements

### Requirement: Two-token authentication

The system SHALL guard management routes (`/api/v1` runtimes, connections,
profiles) with a Mello personal access token (PAT) authenticator, and guard
daemon job routes (`/api/v1/jobs/*`) with a runtime token (`rt_token`)
authenticator. `/webhooks/{provider}` is exempt from both but signature-verified;
`/healthz` is open.

#### Scenario: PAT required for management routes

- **WHEN** a request hits a management route without a valid PAT
- **THEN** the system rejects it as unauthorized

#### Scenario: Runtime token required for job routes

- **WHEN** a daemon calls a job route without a valid `rt_token`
- **THEN** the system rejects it as unauthorized

#### Scenario: Credential is bound to its tenant

- **WHEN** a credential (PAT or runtime token) authenticated for one tenant is used to access another tenant's resource
- **THEN** the system denies the request because the credential only authorizes access to its own tenant's resources
