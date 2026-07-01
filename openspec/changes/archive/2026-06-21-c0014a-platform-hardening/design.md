## Context

The redesign turns mework into a multi-tenant platform. Without per-tenant limits
and a reconstructable audit trail, running this as a shared service is unsafe — one
noisy tenant can starve others, and security-relevant actions (dispatch, grant,
enroll) leave no trace. These two controls are the platform's basic safety
net. They are small focused ports in their own modules so they can be implemented
and tested in isolation, and so the same `Quota` port can be reused by the
scheduler (`c0012`) and the chat surface (`c0011`).

The behaviors are pinned by the e2e scenarios in `tests/e2e/20_quotas_audit_test.go`.

## Goals / Non-Goals

**Goals:**

- A per-tenant `Quota` that admits or rejects an operation against the tenant's
  configured limits, and exposes those limits so an operator UI can render them.
- An `AuditLog` that records security-relevant actions, queryable per tenant in
  append order.
- Atomic concurrency: two concurrent dispatches on the same tenant cannot both
  pass `MaxConcurrentRuns`.

**Non-Goals:**

- Billing / metering UI; the quota port is the enforcement primitive, not a dashboard.
- A separate audit log store (e.g. an external SIEM); the port can be implemented
  over an external sink later, but the default is Postgres.
- Notifications, artifacts, runner selection, secret injection — owned by the
  sibling changes `c0014b` and `c0014c`.

## Decisions

- **`Quota.Allow` returns `(false, nil)` on rejection**, never an error, so the
  caller can choose to queue or reject without unwrapping a sentinel.

- **Atomic `MaxConcurrentRuns`.** A `tenant_active_runs` row is reserved when a
  run is admitted and released when it reaches a terminal state. The reservation
  uses `INSERT ... ON CONFLICT DO NOTHING` so two concurrent transactions cannot
  both reserve when only one slot is free. This is the single source of truth for
  "is the tenant at the limit" — no in-process counter that can drift under
  restarts or multi-instance deploys.

- **Sliding-window dispatch rate.** A `tenant_dispatch_minute` table keyed by
  `(tenant, minute_bucket)` with a count column; `Allow` increments atomically and
  rejects above `MaxDispatchesPerMinute`. The window is bucketed, not a sliding
  log, so the table size stays bounded.

- **Audit log as an append-only table.** `audit_log(tenant, seq, actor, action,
  target, time)` with `(tenant, seq)` as the primary key. `Query` returns rows in
  `seq` order, which is append order by construction. There is no UPDATE/DELETE
  path; entries are immutable.

- **Cross-tenant isolation.** Both ports take `tenant` as a typed argument; the
  SQL binds `tenant` as a parameter, never interpolates from a caller string, and
  the query layer refuses to return rows for a tenant different from the one in
  the auth context.

## Risks

- **Clock skew on the rate window.** A node with skewed time can mis-bucket. The
  bucket is computed from the server's clock and the client does not influence it.
- **Quota table growth.** `tenant_dispatch_minute` rows are pruned by a daily
  job; otherwise the table grows unbounded.
