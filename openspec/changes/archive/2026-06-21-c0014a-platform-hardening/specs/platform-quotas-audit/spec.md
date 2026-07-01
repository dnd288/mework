## ADDED Requirements

### Requirement: Per-tenant quotas and rate limits

The system SHALL enforce **per-tenant** resource limits: a tenant MUST NOT exceed its
configured maximum number of concurrent runs, and dispatches above the tenant's
configured per-minute rate MUST be throttled. A tenant's configured limits SHALL be
queryable through the port.

#### Scenario: Concurrent-run limit is enforced

- **WHEN** a tenant is already at its maximum concurrent runs and another run is dispatched
- **THEN** the dispatch is not admitted (it is rejected or queued until a slot frees)

#### Scenario: Dispatch rate limit is enforced

- **WHEN** a tenant dispatches above its configured per-minute rate
- **THEN** the excess dispatches are throttled rather than admitted

#### Scenario: A tenant's limits are queryable

- **WHEN** an operator queries a tenant's limits
- **THEN** the configured maximum concurrent runs and dispatch rate are returned

### Requirement: Audit log

The system SHALL record **security-relevant actions** (such as agent dispatch, grant
issuance, and runner enrollment) as audit entries carrying actor, action, target,
and time. The audit log MUST be queryable scoped to a single tenant, and entries
MUST be returned in append (chronological) order so the log is tamper-evident.

#### Scenario: Security-relevant actions are logged

- **WHEN** an operator performs a security-relevant action such as a dispatch, grant, or enroll
- **THEN** an audit entry is recorded with the actor, action, target, and time

#### Scenario: Audit log is queryable per tenant

- **WHEN** an operator queries one tenant's audit log
- **THEN** only that tenant's entries are returned and other tenants' entries are not

#### Scenario: Audit entries preserve chronological order

- **WHEN** multiple actions are recorded over time and the audit log is read back
- **THEN** the entries are returned in append (chronological) order

### Requirement: Per-tenant atomic quota enforcement

Quota decisions MUST be made atomically under contention so two concurrent dispatches
on the same tenant cannot both pass the `MaxConcurrentRuns` check.

#### Scenario: Concurrent dispatches respect the run limit

- **WHEN** N concurrent dispatches arrive for a tenant whose `MaxConcurrentRuns` is K (N > K)
- **THEN** exactly K dispatches are admitted and N-K are rejected
