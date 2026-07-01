# Tasks

## 1. Outbound notifications (`server/notify`)

- [ ] 1.1 Define the `Notifier` port (`Notify`) and the `NotifyEvent` shape (kind ∈ {`run.done`,`run.failed`}, run id, target) plus a `DeliveryResult` so retries are observable.
- [ ] 1.2 Implement a Postgres-backed `Notifier` that records delivery attempts and result; expose `Notify(ctx, tenant, event) error`.
- [ ] 1.3 Deliver `run.done` and `run.failed` events to the tenant's configured target URL (HTTP webhook), signing the request with a per-tenant secret.
- [ ] 1.4 Retry on transient failure (5xx / network error) with exponential backoff up to a bounded attempt count; surface a final failure rather than silently dropping the event.
- [ ] 1.5 Cover NOTIFY-01..03: completion fires, failure fires with run id, transient failure retries then succeeds (or surfaces the bounded failure).

## 2. Run artifact store (`server/storage`)

- [ ] 2.1 Define the `ArtifactStore` port (`Put`, `Get`, `List`) over `object-storage`'s `ObjectStore`, with a per-tenant key layout `tenants/{tenant}/runs/{runID}/artifacts/{name}`.
- [ ] 2.2 Implement `Put`: stream bytes to the object store and record a checksum (sha256) in the artifact index so it can be verified on read.
- [ ] 2.3 Implement `Get`: fetch from the object store, recompute and compare against the recorded checksum, return an integrity error on mismatch.
- [ ] 2.4 Implement `List(runID)`: enumerate artifact keys under the run prefix and return metadata (name, size, checksum, created time).
- [ ] 2.5 Generate presigned GET/PUT URLs so a sandbox agent can read or write artifacts without holding store credentials.
- [ ] 2.6 Cover ARTIFACT-01..04: put+get round-trip, list returns all run artifacts, checksum mismatch is detected, agent uses presigned URL (never the store secret).

## 3. Wire run lifecycle to the ports

- [ ] 3.1 On terminal `run.done` / `run.failed` from `c0011-run-events`, call `Notifier.Notify` for the run's tenant (failures of the notifier do not affect the run outcome).
- [ ] 3.2 On run finalization, persist the run's terminal output as an artifact via `ArtifactStore.Put` under the run id.
- [ ] 3.3 Expose `GET /api/v1/runs/{runID}/artifacts` (list) and `GET /api/v1/runs/{runID}/artifacts/{name}` (download via presigned URL).

## 4. Validate

- [ ] 4.1 `go test ./tests/e2e/21_notify_artifacts_test.go` is runnable (currently `t.Skip` on NOTIFY/ARTIFACT); implement the `FakeNotifier` / `FakeArtifactStore` and the per-scenario assertions until all green.
- [ ] 4.2 `make vet` and `make test` stay green.
- [ ] 4.3 `openspec validate c0014b-platform-hardening --type change --strict` passes.
