# Proposal: Per-tenant quotas, rate limits, and audit log

## Why

The hub runs operator-dispatched agents on enrolled runners shared across tenants.
Without per-tenant limits, a single noisy tenant can starve others (unbounded
concurrent runs, unbounded dispatch rate). Without an audit log, security-relevant
actions — dispatch, grant issuance, runner enrollment — leave no reconstructable
trail for forensics or compliance. Neither control exists today.

## What

Introduce two small, focused ports and pin them with the e2e scenarios
`QUOTA-01..03` and `AUDIT-01..03`:

- **Per-tenant quotas (`server/quota`)** — a `Quota` port (`Allow`, `Limits`) that
  admits or rejects an operation against the tenant's `MaxConcurrentRuns` and
  per-minute dispatch rate, and exposes those limits so an operator can query them.
- **Audit log (`server/audit`)** — an `AuditLog` port (`Record`, `Query`) that
  records security-relevant actions with `(actor, action, target, time)` and
  returns entries scoped to a single tenant in append (chronological) order.

## Impact

- **Depends on c0000-tenancy** (every operation is tenant-scoped).
- Module homes: `server/quota` (`Quota`), `server/audit` (`AuditLog`).
- Behaviors are pinned by `tests/e2e/20_quotas_audit_test.go`.

## Capabilities

### New Capabilities

- `platform-quotas-audit`: per-tenant quotas / rate limits with a queryable limits
  surface, and a tenant-scoped append-ordered audit log for security-relevant actions.

## Sibling

This is one of three splits of the original `c0013-platform-hardening`. The other
two are:

- `c0014b-notify-artifacts` — outbound notifications with bounded retry, and a
  run-scoped artifact store.
- `c0014c-selection-secrets` — runner load-balancing + session affinity, and
  grant-scoped secret injection into sandboxes.
