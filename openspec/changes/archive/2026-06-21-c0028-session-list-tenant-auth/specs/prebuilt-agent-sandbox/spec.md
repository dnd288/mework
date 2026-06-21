## MODIFIED Requirements

### Requirement: Remote-control authorization

All session operations (create, attach, send turn, cancel, close, list) SHALL be
authorized: **tenant-isolated**, bound to the session **owner**, and **grant-enforced**
per the `auth-and-secrets` permission model. A caller lacking the required grant or
crossing a tenant boundary MUST be denied. For listing, the tenant scope MUST be derived
from the authenticated caller, never from a caller-supplied argument.

#### Scenario: Cross-tenant access denied

- **WHEN** a caller in tenant `A` attempts to operate on a session in tenant `B`
- **THEN** the system denies the operation

#### Scenario: Operation without grant denied

- **WHEN** a caller attempts a session operation for which it has no permission grant
- **THEN** the system denies the operation

#### Scenario: List is scoped to the caller's own tenant

- **WHEN** a caller lists sessions
- **THEN** the system returns only the authenticated caller's tenant's sessions, regardless of any tenant argument the caller supplies
