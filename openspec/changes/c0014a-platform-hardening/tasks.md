# Tasks

## 1. Per-tenant quotas (`server/quota`)

- [ ] 1.1 Define the `Quota` port (`Allow(ctx, tenant, op) (bool, error)`, `Limits(ctx, tenant) (Limit, error)`) and the `Limit` type (`MaxConcurrentRuns`, `MaxDispatchesPerMinute`).
- [ ] 1.2 Persist per-tenant `Limit` values in Postgres (`tenant_quotas` table) with migration; seed defaults on tenant creation.
- [ ] 1.3 Enforce `MaxConcurrentRuns` atomically using a single SQL `INSERT ... ON CONFLICT DO NOTHING` against a `tenant_active_runs` table so two concurrent dispatches cannot both pass the check (covers the per-tenant atomic quota scenario).
- [ ] 1.4 Enforce `MaxDispatchesPerMinute` via a sliding-window counter in Postgres (`tenant_dispatch_minute` table, minute-bucket key).
- [ ] 1.5 Expose `Limits(ctx, tenant)` so an operator UI can render the active limits.
- [ ] 1.6 Cover QUOTA-01..03: concurrent-run limit, dispatch rate limit, queryable limits.

## 2. Audit log (`server/audit`)

- [ ] 2.1 Define the `AuditLog` port (`Record(ctx, entry) error`, `Query(ctx, tenant, filter) ([]Entry, error)`) and the `Entry` shape (`Actor`, `Action`, `Target`, `Time`, `Tenant`).
- [ ] 2.2 Persist entries to a Postgres `audit_log` table with a `(tenant, seq)` primary key so `Query` can return them in append order without a sort.
- [ ] 2.3 Record security-relevant actions: `dispatch.run`, `grant.issue`, `runner.enroll`, `runner.deactivate`, `quota.update`, `connection.rotate`.
- [ ] 2.4 `Query(ctx, tenant)` returns only that tenant's entries; cross-tenant queries are rejected.
- [ ] 2.5 Cover AUDIT-01..03: action recorded with full entry, per-tenant isolation, append order preserved.

## 3. Wire the controls into the dispatch / enrollment / grant paths

- [ ] 3.1 In `c0004-agent-catalog` dispatch handler, call `Quota.Allow` before enqueuing; on rejection return a typed error the HTTP layer maps to 429.
- [ ] 3.2 In `c0004-agent-catalog` dispatch handler, call `AuditLog.Record` with action `dispatch.run`.
- [ ] 3.3 In `c0004-agent-catalog` grant issuance, call `AuditLog.Record` with action `grant.issue`.
- [ ] 3.4 In `c0005-agent-runner` enrollment handler, call `AuditLog.Record` with action `runner.enroll`.

## 4. Validate

- [ ] 4.1 `go test ./tests/e2e/20_quotas_audit_test.go` is runnable (currently `t.Skip` on QUOTA/AUDIT); implement the `FakeQuota` / `FakeAuditLog` and per-scenario assertions until all green.
- [ ] 4.2 `make vet` and `make test` stay green.
- [ ] 4.3 `openspec validate c0014a-platform-hardening --type change --strict` passes.
