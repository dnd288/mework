## Context

`c0007`–`c0011` give mework multi-tenancy, an SSE bus, an enrolled-runner lifecycle,
pluggable sandboxes, sessions, and an object store. None of these currently tell an
operator **what happened** to a run beyond a single write-back comment, and the run's
output disappears with the sandbox. Notifications and durable artifacts are the two
sides of that gap: a notification fires when a run terminates, and the artifact store
is where the run's output lives after the sandbox is gone.

The contracts are small, focused ports in their own modules, so they can be built and
tested in isolation, and reused by other surfaces (e.g. the scheduler in `c0012`
emits run events; the chat surface in `c0010` writes its outputs as artifacts). The
behaviors are pinned by the e2e scenarios in `tests/e2e/21_notify_artifacts_test.go`.

## Goals / Non-Goals

**Goals:**

- A `Notifier` that delivers `run.done` / `run.failed` per tenant, with bounded retry
  on transient failure so a flapping target cannot silently drop notifications.
- An `ArtifactStore` that persists run outputs over the `object-storage` `ObjectStore`,
  lists them per run, and verifies a recorded checksum on retrieval.
- Sandbox agents read/write artifacts via presigned URLs and never hold store
  credentials.

**Non-Goals:**

- The dispatch / grant / run lifecycle itself (`c0003` / `c0004`) — these ports wrap it.
- A specific notification transport beyond an outbound HTTP webhook target.
- Object-store backend implementation (`c0008`); `ArtifactStore` consumes its port.
- Quotas / audit / selection / secrets — owned by the sibling changes
  `c0014a-quotas-audit` and `c0014c-selection-secrets`.

## Decisions

- **`Notifier.Notify(ctx, tenant, event) error`** with `event.Kind ∈ {"run.done",
  "run.failed"}`, `event.RunID`, `event.Target` (per-tenant URL + signing secret).
  A `DeliveryResult` records attempts, last status, and next-retry time so an operator
  can query delivery state without parsing logs.

- **Retry policy** — bounded exponential backoff (e.g. 4 attempts over ~2 minutes)
  on 5xx / network errors; 4xx is treated as terminal (the target rejected the event).
  Final failure surfaces an error so an operator can see dropped notifications.

- **`ArtifactStore` over `ObjectStore`.** `Put(name, bytes)` writes
  `tenants/{tenant}/runs/{runID}/artifacts/{name}` with a sha256 checksum recorded in
  the artifact index; `Get(name)` recomputes and compares; `List()` enumerates by
  prefix. The runner never sees object-store credentials — sandbox access is via
  presigned GET / PUT URLs minted by the server.

- **Artifact write on run finalization.** The run loop, on terminal state from
  `c0011-run-events`, persists the run's terminal output via `ArtifactStore.Put` and
  fires `Notifier.Notify`. Notifier failures do not roll back the run's terminal
  state — the outbox is durable and the notification retry is independent.

## Risks

- **Webhook target abuse** — a tenant's webhook URL is reachable from the hub. We
  require a per-tenant signing secret and a documented allowlist of acceptable
  targets (HTTPS only by default); callers MUST NOT inject a target from
  attacker-controllable content (provider webhooks).
- **Artifact size** — large artifacts can pressure the object store. The port
  supports streaming, and an upper bound on a single artifact is enforced at the
  server boundary, not at the agent.
