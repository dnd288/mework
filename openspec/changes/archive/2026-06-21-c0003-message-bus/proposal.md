## Why

Today the daemon learns about work by **short-polling** `POST /api/v1/jobs/claim`
on a 5-second ticker (`client/runner`), and the server is a passive REST
endpoint that can never initiate contact. This wastes a request every 5s per
runtime, adds up-to-5s latency before work starts, and cannot support the target
"agent hub" model where the server **publishes** to **topics** that clients
**subscribe** to.

This change introduces a push-based transport: the server becomes a **publisher**
and clients **subscribe over Server-Sent Events (SSE)**. It is the foundation the
`agent-catalog`, `agent-runner`, and `sandbox-runtime` changes build on.

## What Changes

- A new **message-bus** capability: topic-based publish on the server, SSE
  subscription for clients, resumable delivery, and explicit delivery
  acknowledgement.
- The server's broker backend is **pluggable** (default Postgres `LISTEN/NOTIFY`,
  swappable for NATS / in-memory / Redis) — clients only ever see SSE.
- Webhook ingestion **publishes an event to a topic** instead of enqueuing a job
  row as the client-facing transport.
- The Postgres `jobs` table is reframed as one possible **durable backing store
  behind the bus**, not the transport itself; the **client-facing long-poll claim
  is removed**.

## Capabilities

### New Capabilities
- `message-bus`: topic publish/subscribe with an SSE client contract, resumable
  delivery, pluggable server-side broker backend, and delivery acknowledgement.

### Modified Capabilities
- `webhook-pipeline`: inbound events are **published to a topic** rather than
  enqueued as the transport (idempotency preserved).
- `job-queue`: the client-facing **long-poll claim is removed**; the durable job
  store is repositioned as a backing store behind the bus.

## Impact

- **Sequenced after `c0002-repo-restructure`** — builds on the new layout: the SSE
  event schema/DTOs live in `shared/transport`; server code in
  `server/{hub,bus,webhook}`; client code in `client/subscribe`.
- New server routes: `GET /api/v1/.../subscribe` (SSE) and a publish/ack path.
- Affected code: `server/hub` (router), new `server/bus`
  (broker), `server/webhook`, `client/runner` (was the poll
  loop), `client/subscribe` (SSE client).
- New dependency surface: an SSE writer on the server and an SSE client on the
  runner; a broker-backend interface (default Postgres `LISTEN/NOTIFY`).
- Supersedes the 5s poll loop; `agent-runner` consumes this transport.
