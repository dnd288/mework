## 1. Tenant primitive & persistence

- [x] 1.1 Add `Tenant{ID, Name}` and `TenantID` to `server/registry`
- [x] 1.2 Add a `tenants` table migration and a migration assigning existing rows to a default tenant
- [x] 1.3 Add a `tenant_id` column (indexed) to every tenant-scoped resource table

## 2. Tenant registration & scoping

- [x] 2.1 Implement `RegisterTenant(name) → Tenant` allocating a fresh, isolated namespace
- [x] 2.2 Thread `TenantID` through every read/write so queries filter by the caller's tenant
- [x] 2.3 Take the tenant as an explicit argument on listing APIs (e.g. `ListRunners(tenant)`)

## 3. Tenant-scoped tokens & identities

- [x] 3.1 Implement `IssueRegistrationToken(tenant)` recording the owning tenant
- [x] 3.2 Bind the runner identity produced by enrollment to the token's tenant
- [x] 3.3 Reject enrolling/acting across tenants (a token/identity for tenant A cannot reach tenant B)

## 4. Auth integration

- [x] 4.1 Carry the tenant on every authenticated credential (PAT and runner identity)
- [x] 4.2 Scope every authorized request to its credential's tenant; deny cross-tenant access

## 5. Validate

- [x] 5.1 Tests: register an isolated tenant, tenants isolated from each other, registration token scoped to a tenant
- [x] 5.2 openspec validate c0014-tenancy --type change --strict
- [x] 5.3 e2e pointer: flip `tests/e2e/04_runner_enroll_test.go` from Skip to Green for TENANT-01..03; flip `tests/e2e/13_journeys_test.go` E2E-05 (multi-tenant concurrent journeys stay isolated). The cross-tenant isolation claim also gates AUTH-07/08 in `tests/e2e/02_auth_grants_test.go`.
