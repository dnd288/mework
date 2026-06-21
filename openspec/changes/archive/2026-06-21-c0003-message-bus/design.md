## Context

The current transport is Postgres-as-a-queue plus HTTP short-polling: the daemon
calls `POST /api/v1/jobs/claim` every 5s (`client/runner`), which returns
`204` when empty (`client/subscribe`), and the server claims the oldest
queued row with a per-runtime advisory lock and `FOR UPDATE SKIP LOCKED`
(`server/jobs/claim.go`). The server cannot push. The target "agent hub"
needs the server to publish to topics that clients subscribe to.

## Goals / Non-Goals

**Goals:**
- Replace client-side polling with server→client push over SSE.
- Define a topic model and a stable SSE subscription contract.
- Keep the durability substrate pluggable (default Postgres `LISTEN/NOTIFY`).
- Preserve idempotency and at-least-once handling.

**Non-Goals:**
- Choosing a final production broker (NATS/Redis/etc.) — the interface allows it,
  the decision is deferred.
- Agent identity, sessions, enrollment, catalog, or sandboxing — those are the
  `agent-runner`, `agent-catalog`, and `sandbox-runtime` changes.
- Bidirectional streaming: SSE is server→client only; client→server stays POST.

## Decisions

- **SSE, not WebSocket.** Per product direction, clients subscribe only over SSE.
  SSE is simpler (plain HTTP, proxy-friendly, auto-reconnect, `Last-Event-ID`
  resume) and sufficient because the reverse direction (ack/result/pull) is
  ordinary POST/GET.
- **Topic naming.** Hierarchical, dot-delimited (e.g. `runner.<id>.dispatch`,
  `session.<id>.control`). Routing to a specific runner = publishing to its topic.
- **Pluggable backend behind an interface.** Default Postgres `LISTEN/NOTIFY`
  (no new infra; reuses the DB), with the durable `jobs`/message table providing
  resume and redelivery. The interface (`Publish`, `Subscribe(from eventID)`,
  `Ack`) lets NATS/Redis/in-memory drop in without touching the SSE layer.
- **At-least-once + idempotent consumers.** Delivery is at-least-once; messages
  carry a stable id so consumers dedupe. Acks are POST; unacked leased messages are
  redeliverable.
- **Resume semantics.** Event ids are monotonic per stream; `Last-Event-ID` drives
  resume, bounded by backing-store retention.

## Risks / Trade-offs

- **SSE connection scaling.** Many long-lived connections per server instance;
  mitigated by connection limits and horizontal scale (a shared backend like NATS
  if Postgres `LISTEN/NOTIFY` connection fan-out becomes a ceiling).
- **Postgres `LISTEN/NOTIFY` limits.** Payload size and per-connection fan-out are
  bounded; acceptable for the default tier, and the pluggable interface is the
  escape hatch.
- **Proxy/idle timeouts** can drop SSE streams; mitigated by heartbeat comments and
  `Last-Event-ID` resume.
- **Ordering** is per-topic best-effort, not global; consumers must not assume
  cross-topic ordering.
