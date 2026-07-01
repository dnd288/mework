## Context

The hub is moving from a single-account poll/queue model toward a multi-customer
agent platform. Many of the proposed capabilities — catalog, runners, dispatch,
sessions, schedules, storage/workspaces — are inherently per-customer, and several
of their interfaces already thread a `TenantID` (e.g. `Registry.ListRunners`,
`Scheduler.List`, `SessionManager.List`, `WorkspaceSpec.RemotePrefix` keyed by
`<tenant>/<session>/`). But there is no tenant primitive yet: `server/registry` has
no `Tenant` type and registration tokens are not bound to an owner. This change
establishes that boundary so the rest of the platform can rely on it.

## Goals / Non-Goals

**Goals:**
- A first-class **tenant** boundary owned by `server/registry`.
- Register an isolated tenant and key every hub resource by its tenant.
- Isolate tenants from each other: cross-tenant reads/writes are denied.
- Bind registration tokens (and the runner identities they produce) to a tenant.

**Non-Goals:**
- Per-tenant **quotas/limits** and audit — those are separate platform-hardening
  concerns (`Quota`, `AuditLog`).
- Runner **enrollment** mechanics and the durable runner identity lifecycle — that is
  `agent-runner`; tenancy only requires the token and identity be tenant-bound.
- Cross-tenant **sharing** of agents/artifacts (a future, opt-in concern).

## Decisions

- **`Tenant{ID, Name}` is the boundary primitive.** `TenantID` is a stable,
  opaque identifier; `Name` is the human label given at registration. The tenant is
  the single unit of isolation.
- **`RegisterTenant(name) → Tenant` creates an isolated tenant.** It allocates a new
  `TenantID` and a fresh, empty namespace; no resources are shared with any existing
  tenant.
- **Every resource is keyed by tenant.** Catalog entries, runners, dispatches,
  sessions, schedules, and stored objects each carry a `TenantID`; all reads and
  writes are filtered by the caller's tenant. There is no global, un-scoped view of
  tenant resources. Listing APIs (e.g. `ListRunners(tenant)`) take the tenant as an
  explicit argument.
- **Registration tokens are scoped to a tenant.** `IssueRegistrationToken(tenant)`
  records the owning `TenantID`; enrolling with that token yields a runner identity
  bound to the same tenant. A token issued for tenant A can never enroll a runner into
  tenant B.
- **Cross-tenant access is denied by construction.** A credential (PAT or runner
  identity) carries its tenant; the server scopes every query by that tenant, so a
  request scoped to tenant A never observes or mutates tenant B's state — the other
  tenant's resources are simply invisible, not merely forbidden.

## Risks / Trade-offs

- **Pervasive change.** Tenant keying touches every persistence table and query; a
  single missed `tenant_id` filter is a cross-tenant leak. Mitigate with a shared,
  enforced scoping helper and tests that assert isolation.
- **Migration of existing single-account state.** Existing rows must be assigned to a
  default tenant on migration; the mapping must be explicit and reversible.
- **Token/identity binding integrity.** The tenant binding on a registration token and
  on a runner identity must be tamper-resistant (reuse the `token`/`secret`
  primitives) so a runner cannot rebind itself to another tenant.
- **Performance.** Every query gains a tenant predicate; ensure `tenant_id` is indexed
  on each scoped table.
