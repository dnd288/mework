# Architecture — Agent Hub, Runners & Sandboxes

> This is the canonical architecture document. It describes the **target**
> architecture — the agent hub — as the product's intended shape, and maps every part
> back to **what runs today**. Status badges throughout: **`[Implemented]`** ships in
> the current code; **`[Planned — cNNNN]`** is specified under `openspec/changes/` and
> not yet built.
>
> **Bottom line on status:** everything in the "agent hub" sections is **planned**.
> The code today implements the poll/queue pipeline described under
> [Implementation today](#implementation-today). Only `c0006` (a parse fix) of the
> redesign changes has shipped.

## Two binaries

Go module `mework` (Go 1.25.7), two binaries that share `internal/` packages:

- **`cmd/mework`** — the CLI **and** the local agent daemon/runner (client side).
- **`cmd/mework-server`** — the central provider-gateway HTTP server (server side).

## Why the redesign

The current transport is **pull-based**: the daemon short-polls
`POST /api/v1/jobs/claim` every ~5 seconds and the server can never initiate contact.
That wastes requests, adds up to 5s of latency, and cannot support an "agent hub" that
pushes work to subscribed clients. Agents also run **unsandboxed** today (a bare host
subprocess), which is unsafe for running operator-dispatched agents.

The redesign adopts the **DX of a GitHub Actions self-hosted runner**: install a
runner once, then drive everything remotely from the hub — pull new agents on demand,
run any *permitted* operation — without ever manually operating on the client machine
again.

## The three components (target)

### 1. Server = Agent Hub  `[Planned — c0002/c0003]`
The central, provider-agnostic brain:
- **Registry & sessions** — runners enroll and hold long-lived sessions; the hub
  tracks presence over the SSE channel.
- **Publisher / message broker** — publishes messages to **topics**; the broker
  backend is pluggable (default Postgres `LISTEN/NOTIFY`; swappable for NATS, Redis,
  in-memory for tests) and never exposed to clients directly.
- **Agent catalog** — agents are **versioned, pullable artifacts**: a
  definition/manifest *or* a packaged/container image (`form` field).
- **Orchestrator** — routes/dispatches an agent to the right runner by publishing to
  its topic.
- **Permission/policy engine** — issues scoped, least-privilege **grants** that travel
  with each dispatch.

### 2. Daemon = Runner  `[Planned — c0004]`
The client on a local device, modeled on `actions/runner`:
- **Enroll once** — exchange a hub URL + short-lived registration token for a durable
  runner identity; thereafter **unattended**.
- **Subscribe over SSE** — hold one Server-Sent Events stream; receive dispatches by
  **push**, never by polling. Reconnect with jittered backoff + `Last-Event-ID` resume.
- **Pull → run → report** — on dispatch, pull the referenced agent version from the
  catalog, run it in a sandbox, report the result over POST, acknowledge the dispatch.
- **Enforce grants** — refuse operations outside the dispatched grant, locally.
- **Manage local sandboxes** — own the lifecycle of the sandboxes on its device.

### 3. Sandbox = isolated agent runtime  `[Planned — c0005]`
- **Pluggable drivers** — `local` (host subprocess, current behavior, trusted use) and
  `docker` (a container per agent), extensible to other isolation backends.
- **One agent per sandbox**, created for the run and destroyed after (cleanup
  guaranteed even on failure).
- **Isolation + limits** — isolating drivers confine the agent from the host
  filesystem/network/env and enforce CPU/memory/timeout limits. The prompt is always
  fed over **stdin, never argv**.

See [runtime-and-sandbox.md](runtime-and-sandbox.md) for the runner loop and the
sandbox `Driver` interface in detail.

## Data flow (target)

```
provider ─webhook→ Server (Agent Hub)
                     • catalog: versioned, pullable agents
                     • registry + permission/policy engine
                     • publisher/broker: topics (pluggable backend)
                     • orchestrator + session manager
                     │  SSE stream  (server ─push→ runner)
                     ▼
                   Daemon = Runner  (enrolled once, unattended)
                     • subscribe topics over SSE
                     • on dispatch: PULL agent  ─GET→ catalog
                     • enforce granted permissions
                     │ spawn (one agent per sandbox)
                     ▼
                   Sandbox (local | docker | …)
                     • isolated; runs ONE agent
                     • result ─POST→ hub ─REST writeback→ provider
```

## Client contract: SSE only  `[Planned — c0002]`

Clients **subscribe only over Server-Sent Events** (`text/event-stream`). The server
pushes events as work is published; each event has a monotonic id, so a reconnecting
client resumes with `Last-Event-ID`. The reverse direction — acknowledgements,
results, and agent pulls — is ordinary POST/GET. The server's internal broker (queue,
stream, or DB) is an implementation detail behind this contract.

**Topics** are hierarchical and dot-delimited: `runner.<id>.dispatch`,
`session.<id>.control`. Routing work to a specific runner means publishing to its
topic. Delivery is **at-least-once with idempotent consumers** (each message carries a
stable id for dedupe); unacked leased messages are redeliverable until ack or lease
expiry; per-topic best-effort ordering, no global ordering.

The single source of truth for this wire contract is the planned
`internal/shared/transport` package (SSE event schema + API DTOs), depended on by both
client and server — see [c0001 restructure](#roadmap).

## Permission model: "any *permitted* operation"  `[Planned — c0003]`

Every dispatch carries a **scoped, least-privilege grant**. Three layers of defense so
a buggy agent or a compromised message cannot exceed its scope:

1. **Hub authorizes** — issues a signed/sealed grant scoped to this run.
2. **Runner enforces locally** — verifies the grant's integrity and refuses operations
   outside scope; it cannot widen its own grant.
3. **Sandbox contains** — the isolation boundary stops anything the runner doesn't
   mediate.

No grant for an operation means that operation is denied. Grants are scoped **per run,
not per identity** — the same runner can be highly privileged for one dispatch and
minimal for the next. See [auth-and-secrets.md](auth-and-secrets.md).

## Today → Target migration

| Concern | Today `[Implemented]` | Target `[Planned]` | OpenSpec change |
|---|---|---|---|
| Transport | 5s poll of `/jobs/claim` | SSE subscribe + topic publish | `c0002-message-bus` |
| Server role | passive REST endpoint | publisher / broker / hub | `c0002-message-bus` |
| Work routing | claim oldest queued row | publish to a runner/session topic | `c0002-message-bus` |
| Agent definition | static `profiles` row | versioned, pullable catalog artifact | `c0003-agent-catalog` |
| Distribution | none (profile snapshot in job) | pull agent on dispatch | `c0003-agent-catalog` |
| Permissions | none (implicit trust) | scoped grant per dispatch | `c0003-agent-catalog` |
| Client identity | pre-registered `runtime` + `rt_token` | install-once enrollment + runner identity | `c0005-agent-runner` |
| Client loop | poll worker | enrolled SSE pull→run→report | `c0005-agent-runner` |
| Execution | bare host subprocess | pluggable sandbox driver | `c0005-sandbox-runtime` |
| Isolation | a `0700` directory | container / driver isolation + limits | `c0005-sandbox-runtime` |

**Reused as-is (orthogonal to the redesign):** the provider-gateway adapter registry,
the REST write-back outbox, webhook ingestion (becomes *publish* instead of
*enqueue*), and the auth/token/secret primitives.

## Roadmap

The redesign lands as five OpenSpec changes, in dependency order:

0. **`c0002-repo-restructure`** — **foundational, lands first.** A pure mechanical
   refactor (zero behavior change) into `shared` / `client` / `server` / `platform`
   domains with an enforced one-way dependency rule, per-component build/test, and a
   single home (`internal/shared/transport`) for the client↔server wire contract — so
   the features below can be built, tested, and released independently and in parallel.
1. **`c0002-message-bus`** — SSE pub/sub transport (foundation). Replaces the long-poll
   claim with topic subscribe; reframes `jobs` as the durable backing store behind the
   bus.
2. **`c0003-agent-catalog`** — pullable versioned agents (`publish`/`pull`/`dispatch`)
   + the permission/policy grant model. Extends the auth model with a runner identity.
3. **`c0005-agent-runner`** — install-once enrollment + the SSE pull→run→report loop;
   local grant enforcement.
4. **`c0005-sandbox-runtime`** — pluggable isolated execution drivers (`local`/`docker`).

Status: **`c0001`–`c0005` are proposed, none implemented.** `c0006-normalize-workflow-keyword`
(an unrelated parse-normalization hardening) is the only shipped redesign-era change.

The `internal/` target layout after `c0001`:

```
internal/
  shared/     core types · transport (wire contract) · config · providers/mello · errors · log
  client/     cli · runner · subscribe (SSE) · sandbox (drivers)
  server/     hub · registry · session · catalog · bus · orchestrator · permission
              webhook · writeback · provider/mello · auth · middleware
  platform/   store (Postgres) · secret (AES) · token (HMAC)
```

Dependency DAG: `shared` (leaf) ← `platform` ← `server`; `shared` ← `client`;
**`client ⟂ server` (never import each other)**. Enforced in CI via an import-guard
lint. Change directories are named `cNNNN-<slug>` to encode apply order; see
[openspec-workflow.md](openspec-workflow.md).

---

## Implementation today

This is the **current, shipped** architecture (`[Implemented]`) — the code you are
working in until the redesign lands.

### End-to-end flow

```
Mello (kanban)
  │  user comments "@mework <profile> [workflow] <instructions>" on a ticket
  ▼
POST /webhooks/{provider}        (mework-server)
  │  adapter verifies signature, ParseTrigger matches the grammar
  ▼
jobs.Enqueue  ──▶  Postgres `jobs` (status=queued, deduped on provider_code+external_event_id)
  ▲                                   │
  │ rt_token auth                     │ long-poll claim (FOR UPDATE SKIP LOCKED)
  ▼                                   ▼
mework daemon  ──▶ claim → ack running → heartbeat (30s) → run AI CLI → ack done/failed
  │                                   (prompt via STDIN, isolated workdir, 30m timeout)
  ▼
server: durable outbox  ──▶  provider REST API (e.g. Mello CreateComment)  ──▶ result posted back
```

### Package map

| Path | Responsibility |
|------|----------------|
| `cmd/mework/` | CLI + daemon entrypoint; cobra commands `cmd_*.go` (board, ticket, auth, provider, runtime, profile, daemon, version) |
| `cmd/mework-server/` | Server entrypoint: load config → run migrations → start chi HTTP server with graceful shutdown |
| `internal/cli/` | Config struct & persistence (`~/.mework/config.json`), flag/env/file resolution, profile paths |
| `internal/mello/` | Mello REST API client + models |
| `internal/meworkclient/` | HTTP client for `mework-server` (jobs claim/ack/heartbeat, connections, profiles, runtimes) |
| `internal/daemon/` | Daemon lifecycle (pid/health), poll loop, prompt building & result formatting |
| `internal/agentrun/` | Detects installed AI CLIs and executes them (prompt via stdin, isolated workdir) |
| `internal/store/` | Postgres pgx pool + embedded goose migrations |
| `internal/server/` | chi router, config, `/healthz` |
| `internal/server/{auth,middleware}/` | PAT and runtime (`rt_token`) authenticator middleware |
| `internal/server/{registry,connection,profile}/` | Runtimes / provider-connection / AI-profile CRUD |
| `internal/server/webhook/` | `/webhooks/{provider}` handler, signature verify, `ParseTrigger`, enqueue |
| `internal/server/jobs/` | Job lifecycle: enqueue, claim, ack, heartbeat, state machine, sweeper, write-back |
| `internal/server/provider/` | Provider adapter interface + registry; `provider/mello/` is the first adapter |
| `internal/server/{secret,token}/` | AES-256-GCM seal/unseal; runtime token generation + HMAC-SHA256 lookup hashing |
| `internal/integration/` | End-to-end pipeline test |

For endpoints, the wire schema, and the database tables, see
[api-reference.md](api-reference.md). For the runner loop and execution model, see
[runtime-and-sandbox.md](runtime-and-sandbox.md). For tokens and sealing, see
[auth-and-secrets.md](auth-and-secrets.md).
