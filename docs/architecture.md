# Architecture — Agent Hub, Runners & Sandboxes

> This is the canonical architecture document. It describes the agent-hub architecture and
> maps each part to **what runs today**. Status badges: **`[Implemented]`** ships in the
> current code; **`[Planned]`** is specified but not yet built.
>
> **Bottom line on status:** the agent hub is **substantially implemented** — the repo
> restructure, install-once runner enrollment, SSE-pushed dispatch, the agent catalog +
> grants, interactive sessions, and workspace-bound sandboxes (`local`/`docker` engines) all
> ship. The **legacy poll/queue pipeline** (webhook → job → claim → write-back) also ships and
> is the **default** webhook path. Remaining stubs: artifact store, NATS bus, GitHub/Jira
> providers, the standalone `mework-sandbox` binary, and cloudflare/custom engines.

## Binaries and modules

Go module `mework` (Go 1.26), a `go.work` workspace. Binaries live under `apps/`; shared code
under `libs/{client,server,shared,sandbox,tests,tools}` (the `cmd/` + `internal/` layout was
replaced by the restructure):

- **`apps/mework`** — the CLI, the local agent daemon/runner, **and** `mework server start`
  (run the hub in-process). Client-side packages: `libs/client/{cli,runner,enroll,subscribe,
  catalog,workspacefs}`.
- **`apps/mework-server`** — the standalone provider-gateway HTTP server. Server-side packages:
  `libs/server/{hub,auth,middleware,registry,connection,catalog,session,bus,orchestrator,
  webhook,writeback,channel,provider,platform,storage}`.
- **`apps/mework-mezon-worker`** — the standalone Mezon worker binary. Runs as a separate
  process: the inbound loop receives Mezon channel messages via WebSocket and enqueues them
  via `POST /api/v1/jobs/enqueue`; the outbound loop independently polls for completed jobs
  and posts replies back to Mezon channels via the bot client.
- **`libs/sandbox`** — pluggable engines (`local`/`docker`/cloudflare/custom) + runtime
  manager; `libs/sandbox/cmd/mework-sandbox` is a standalone runner *(stub today)*.
- **`libs/shared`** — `core` types, `transport` wire contract, `config`, `grant`, `providers`.

## Why the redesign

The original transport was **pull-based**: the daemon short-polled
`POST /api/v1/jobs/claim` every ~5 seconds and the server could never initiate contact —
wasteful, up to ~5s of latency, and unable to support an "agent hub" that pushes work to
subscribed clients. (The legacy poll/claim path still ships as the default webhook pipeline;
the SSE push transport below now coexists with it.) The redesign also moved execution into
**pluggable sandboxes** (`local`/`docker`), so dispatched agents no longer run as a bare host
subprocess.

The redesign adopts the **DX of a GitHub Actions self-hosted runner**: install a
runner once, then drive everything remotely from the hub — pull new agents on demand,
run any *permitted* operation — without ever manually operating on the client machine
again.

## The three components

### 1. Server = Agent Hub  `[Implemented]`
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

### 2. Daemon = Runner  `[Implemented]`
The client on a local device, modeled on `actions/runner`:
- **Enroll once** — exchange a hub URL + short-lived registration token for a durable
  runner identity; thereafter **unattended**.
- **Subscribe over SSE** — hold one Server-Sent Events stream; receive dispatches by
  **push**, never by polling. Reconnect with jittered backoff + `Last-Event-ID` resume.
- **Pull → run → report** — on dispatch, pull the referenced agent version from the
  catalog, run it in a sandbox, report the result over POST, acknowledge the dispatch.
- **Enforce grants** — refuse operations outside the dispatched grant, locally.
- **Manage local sandboxes** — own the lifecycle of the sandboxes on its device.

### 3. Sandbox = isolated agent runtime  `[Implemented]` (local/docker; cloudflare/custom partial)
- **Pluggable drivers** — `local` (host subprocess, current behavior, trusted use) and
  `docker` (a container per agent), extensible to other isolation backends.
- **One agent per sandbox**, created for the run and destroyed after (cleanup
  guaranteed even on failure).
- **Isolation + limits** — isolating drivers confine the agent from the host
  filesystem/network/env and enforce CPU/memory/timeout limits. The prompt is always
  fed over **stdin, never argv**.

See [runtime-and-sandbox.md](runtime-and-sandbox.md) for the runner loop and the
sandbox `Driver` interface in detail.

## Data flow

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

## Client contract: SSE only  `[Implemented]`

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

The single source of truth for this wire contract is the
`libs/shared/transport` package (SSE event schema + API DTOs), depended on by both
client and server.

## Permission model: "any *permitted* operation"  `[Implemented]`

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

## Legacy pipeline → agent hub (both ship)

Both columns are **implemented today**: the legacy pipeline is the default webhook path, and
the agent-hub mechanisms run alongside it. The table maps the evolution per concern.

| Concern | Legacy pipeline | Agent hub | Status |
|---|---|---|---|
| Transport | 5s poll of `/jobs/claim` | SSE subscribe + topic publish | both ship |
| Server role | passive REST endpoint | publisher / broker / hub | both ship |
| Work routing | claim oldest queued row | publish to a runner/session topic | both ship |
| Agent definition | static `profiles` row | versioned, pullable catalog artifact | both ship |
| Distribution | none (profile snapshot in job) | pull agent on dispatch | both ship |
| Permissions | none (implicit trust) | scoped grant per dispatch | both ship |
| Client identity | pre-registered `runtime` + `rt_token` | install-once enrollment + runner identity | both ship |
| Client loop | poll worker | enrolled SSE pull→run→report | both ship |
| Execution | bare host subprocess | pluggable sandbox driver | both ship |
| Isolation | a `0700` directory | container / driver isolation + limits | both ship |

**Reused as-is (orthogonal to the redesign):** the provider-gateway adapter registry,
the REST write-back outbox, webhook ingestion (becomes *publish* instead of
*enqueue*), and the auth/token/secret primitives.

## Roadmap

The redesign shipped as a sequence of OpenSpec changes (archived under
`openspec/changes/archive/`), in dependency order:

0. **`c0002-repo-restructure`** — **foundational, lands first.** A pure mechanical
   refactor (zero behavior change) into `shared` / `client` / `server` / `platform`
   domains with an enforced one-way dependency rule, per-component build/test, and a
   single home (`libs/shared/transport`) for the client↔server wire contract — so
   the features below can be built, tested, and released independently and in parallel.
1. **`c0002-message-bus`** — SSE pub/sub transport (foundation). Replaces the long-poll
   claim with topic subscribe; reframes `jobs` as the durable backing store behind the
   bus.
2. **`c0003-agent-catalog`** — pullable versioned agents (`publish`/`pull`/`dispatch`)
   + the permission/policy grant model. Extends the auth model with a runner identity.
3. **`c0005-agent-runner`** — install-once enrollment + the SSE pull→run→report loop;
   local grant enforcement.
4. **`c0005-sandbox-runtime`** — pluggable isolated execution drivers (`local`/`docker`).

Status: **shipped.** The restructure, message bus, agent catalog + grants, runner enrollment,
and sandbox runtime all landed (archived under `openspec/changes/archive/`). The realized
layout (note: `libs/`, not `internal/`):

```
libs/
  shared/     core types · transport (wire contract) · config · grant · providers/mello
  client/     cli · runner · subscribe (SSE) · catalog (resolvers) · enroll · workspacefs
  server/     hub · registry · session · catalog · bus · orchestrator · channel
              webhook · writeback · provider/mello · auth · middleware · platform · storage
  sandbox/    engine/{local,docker,cloudflare,custom} · runtime · agent · cmd/mework-sandbox
```

Dependency DAG: `shared` (leaf) ← `server`; `shared`,`sandbox` ← `client`;
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
| `apps/mework/` | CLI + daemon entrypoint + `server start` (wires the in-process hub via `cli.SetServerStarter`) |
| `apps/mework-server/` | Standalone server entrypoint: load config → migrate → chi server with graceful shutdown |
| `apps/mework-mezon-worker/` | Standalone Mezon worker: inbound loop receives WebSocket messages and enqueues jobs via the server API; outbound loop polls for completed jobs and posts replies to Mezon |
| `libs/client/cli/` | cobra commands `cmd_*.go` (board, ticket, workspace, daemon, runner, runtime, agent, session, sandbox, server, profile, provider, auth, config, version) + config persistence (`~/.mework/`) |
| `libs/client/runner/` | daemon lifecycle, SSE `Engine` (dispatch loop), one-shot + interactive `Session` execution |
| `libs/client/{enroll,subscribe,catalog,workspacefs}/` | runner enrollment, SSE client, definition resolvers (HTTP/file), workspace artifact I/O |
| `libs/shared/{core,transport,config,grant,providers/mello}/` | core types, wire contract, config, grants, Mello REST client |
| `libs/server/hub/` | chi router, config (env), `/healthz` `/livez` `/readyz`, server assembly |
| `libs/server/{auth,middleware}/` | PAT and runtime (`rt_token`) + grant authenticator middleware |
| `libs/server/{registry,connection,catalog}/` | runtimes/enrollment, provider connections, agent catalog + profiles |
| `libs/server/{session,bus,orchestrator,channel}/` | session manager, message broker (memory/postgres), job lifecycle, channel routing |
| `libs/server/{webhook,writeback,provider}/` | webhook handler + `ParseTrigger`, REST write-back outbox, provider adapter registry (`provider/mello`) |
| `libs/server/platform/{store,secret,token}/` | Postgres pool + goose migrations; AES-256-GCM seal/unseal; HMAC token hashing |
| `libs/sandbox/` | engines (`local`/`docker`/cloudflare/custom), runtime manager, agent detection |
| `libs/tests/{integration,e2e}/` | DB-backed integration tests; `e2e` BDD suite (behind the `e2e` build tag) |

For endpoints, the wire schema, and the database tables, see
[api-reference.md](api-reference.md). For the runner loop and execution model, see
[runtime-and-sandbox.md](runtime-and-sandbox.md). For tokens and sealing, see
[auth-and-secrets.md](auth-and-secrets.md).
