## ADDED Requirements

### Requirement: Register an isolated tenant

The hub SHALL let an operator register a **tenant** as an isolated boundary. Each
tenant MUST have a stable identifier, and registering a tenant MUST create a fresh,
empty namespace that scopes all of that tenant's catalog, runner, dispatch, session,
schedule, and storage state.

#### Scenario: Operator registers a tenant

- **WHEN** an operator registers a tenant named `acme`
- **THEN** the hub creates a tenant with a stable identifier and a boundary that scopes all of `acme`'s catalog/runner/dispatch state

### Requirement: Tenants are isolated from each other

The hub SHALL isolate tenants so that an identity scoped to one tenant MUST NOT see or
act on another tenant's resources. Every tenant-scoped resource MUST be keyed by its
tenant, and every read or write MUST be filtered by the caller's tenant so that
cross-tenant access is denied.

#### Scenario: Listing returns only the caller's tenant

- **WHEN** an identity scoped to tenant `acme` lists runners while tenant `globex` also has runners
- **THEN** only `acme`'s runners are returned and `globex`'s are never visible

#### Scenario: Cross-tenant access is denied

- **WHEN** an identity scoped to tenant `acme` attempts to read or act on a resource owned by tenant `globex`
- **THEN** the hub denies the access because the resource is outside the caller's tenant boundary

### Requirement: Registration tokens are scoped to a tenant

The hub SHALL bind every registration token to a single tenant. Issuing a registration
token MUST record its owning tenant, and enrolling with that token MUST yield a runner
identity bound to the same tenant; a token issued for one tenant MUST NOT enroll a
runner into any other tenant.

#### Scenario: Issued token is bound to its tenant

- **WHEN** an operator issues a registration token for tenant `acme`
- **THEN** the token records `acme` as its owning tenant

#### Scenario: Enrolling yields a tenant-bound identity

- **WHEN** a runner enrolls using a registration token issued for tenant `acme`
- **THEN** the resulting runner identity is bound to tenant `acme` and cannot reach any other tenant's resources
