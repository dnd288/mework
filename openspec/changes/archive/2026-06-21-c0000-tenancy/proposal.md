## Why

The hub is a multi-customer platform: one deployment serves many independent
organizations, each with its own runners, agents, dispatches, sessions, schedules,
and stored artifacts. Without a tenant boundary, an identity from one organization
could see or act on another's state, and a registration token issued for one
organization could enroll a runner into the wrong one. Today there is no notion of a
tenant: `server/registry` has no `Tenant`, no `RegisterTenant`, and registration
tokens are not bound to any owner.

This change introduces **multi-tenant isolation** as a foundational boundary: every
catalog/runner/dispatch/session/schedule/storage resource is keyed by a tenant, and
registration tokens (and the runner identities they yield) are bound to a tenant so
cross-tenant access is denied by construction.

## What Changes

- A new **tenancy** capability in `server/registry`: an operator can **register an
  isolated tenant** (`RegisterTenant`), and every resource the hub manages is scoped
  to exactly one tenant via its `TenantID`.
- A **tenant boundary** that isolates tenants from each other: an identity scoped to
  one tenant can only see and act on that tenant's state; cross-tenant reads and
  writes are denied.
- **Tenant-scoped registration tokens**: `IssueRegistrationToken` is bound to a
  tenant, and enrolling with such a token yields a runner identity bound to that same
  tenant.

## Capabilities

### New Capabilities
- `tenancy`: register an isolated tenant, scope all catalog/runner/dispatch/session/
  schedule/storage state to one tenant, and bind registration tokens (and the runner
  identities they yield) to a tenant so cross-tenant access is denied.

### Modified Capabilities
- `auth-and-secrets`: tenant scoping — every authenticated credential (PAT and runner
  identity) is bound to a tenant, and the credential only authorizes access to its own
  tenant's resources.

## Impact

- **Foundational.** Downstream capabilities (`agent-catalog`, `agent-runner`,
  `message-bus`, sessions, scheduling, storage/workspaces) assume a per-tenant
  boundary; every per-tenant requirement elsewhere depends on this change. Tenancy
  must land before those reqs can be enforced.
- Home module is `server/registry`: adds `Tenant{ID,Name}`, `TenantID`,
  `RegisterTenant`, and a tenant argument on `IssueRegistrationToken` (and on listing
  APIs such as `ListRunners`).
- New persistence: a `tenants` table, plus a `tenant_id` key on every tenant-scoped
  resource; queries filter by tenant. Registration tokens record their owning tenant.
- Behaviors covered: **TENANT-01** (register an isolated tenant), **TENANT-02**
  (tenants are isolated from each other; cross-tenant access denied), **TENANT-03**
  (registration tokens are scoped to a tenant).
