# Proposal: Outbound notifications and durable run artifacts

## Why

The hub runs operator-dispatched agents on enrolled runners, but once a run ends the
operator has no out-of-band signal that it finished or failed (today's poll pipeline
only writes a comment back to the provider), and the run's output disappears with the
sandbox. An operator needs to know when runs finish or fail without polling, and to
recover a run's outputs after the sandbox is gone — both **per-tenant, durable, and
portable** so that the operator can wire them into their own systems.

## What

Introduce two small, focused ports and pin them with the e2e scenarios
`NOTIFY-01..03` and `ARTIFACT-01..04`:

- **Outbound notifications (`server/notify`)** — a `Notifier` port (`Notify`,
  `NotifyEvent`) that delivers `run.done` and `run.failed` events to a tenant's
  configured target, with bounded delivery retry on transient failure so a flapping
  target does not silently drop notifications.
- **Run artifact store (`server/storage`)** — an `ArtifactStore` port (`Put`,
  `Get`, `List`) that persists a run's outputs over the `c0008` object store,
  scoped per run, listable per run, and integrity-checked against a recorded
  checksum on retrieval.

## Impact

- **Depends on c0000-tenancy** (per-tenant target / per-tenant scoping).
- **Depends on c0008-object-storage** for the `ObjectStore` backend that
  `ArtifactStore` persists artifacts to.
- **Depends on c0011-run-events** (the `run.done` / `run.failed` events the notifier
  consumes).
- Module homes: `server/notify` (`Notifier`), `server/storage` (`ArtifactStore`).
- Behaviors are pinned by `tests/e2e/21_notify_artifacts_test.go`.

## Capabilities

### New Capabilities

- `platform-notify-artifacts`: outbound notifications with bounded retry, and a
  run-scoped artifact store with checksum integrity, over the existing object-store
  port.

## Sibling

This is one of three splits of the original `c0013-platform-hardening`. The other
two are:

- `c0014a-quotas-audit` — per-tenant quotas/rate limits and the tenant-scoped audit
  log.
- `c0014c-selection-secrets` — runner load-balancing + session affinity, and
  grant-scoped secret injection into sandboxes.
