# API Reference

> Audience: developers integrating with or extending the mework hub. Covers HTTP
> endpoints, authentication per route, the SSE bus + catalog + session API, and the data
> model. Status badges: **`[Implemented]`** exists today; **`[Planned]`** is specified but
> not yet built.
>
> Source of truth: `libs/server/hub/router.go` (routes), `libs/server/platform/store/migrations/`
> (schema), `libs/server/webhook/parse.go` (trigger grammar).

## Authentication schemes

| Scheme | Credential | Guards | Mechanism |
|--------|-----------|--------|-----------|
| **open** | none | `/healthz`, `/livez`, `/readyz`, `/webhooks/{provider}` | — (webhooks are signature-verified inside the handler) |
| **signature** | per-connection webhook secret | `/webhooks/{provider}` | HMAC-SHA256 verified inside the handler |
| **rt_token (runner identity)** `[Implemented]` | runtime bearer token (`mework_rt_…`), obtained via `runner enroll` | `/api/v1/jobs/*`, `/api/v1/runners/sessions/{id}/*`, agent pull | HMAC-SHA256 lookup hash keyed by `SERVER_KEY` |
| **PAT** `[Implemented]` | Mello personal access token | `/api/v1` management + session routes | bearer → Mello `/me`, cached 60s |
| **registration token** `[Implemented]` | one-time, short-lived token | `POST /api/v1/runners/enroll` | issued by `POST /api/v1/runners/registration-tokens` (PAT) |
| **grant** `[Implemented]` | signed, scoped capability | agent pull (`OpPullAgent`), spawn (`OpSpawn`) | verified by `GrantMiddleware` keyed by `SERVER_KEY` |

See [auth-and-secrets.md](auth-and-secrets.md) for token formats, hashing, and the
grant model.

## HTTP endpoints — current `[Implemented]`

Global middleware (chi): `RequestID`, `RealIP`, `Logger`, `Recoverer`.

### Health & webhooks

| Method | Path | Auth | Handler / behavior |
|--------|------|------|--------------------|
| GET | `/healthz` | open | DB ping → `200 {"status":"ok"}` or `503 {"status":"not ready"}` (no error leak) |
| GET | `/livez` | open | Process liveness, **DB-independent** → always `200` |
| GET | `/readyz` | open | Readiness (DB ping) → `200`/`503`, generic body |
| POST | `/webhooks/{provider}` | signature | Verify, parse trigger, enqueue. `202` on enqueue; `200` on any silent-ignore path (so the provider stops retrying); `401` only on missing/failed signature |

The server also caps request bodies (`RequestSize`, 4 MiB) and sets `ReadHeaderTimeout`/
`IdleTimeout` (SSE-safe — no `WriteTimeout`).

### Runtime routes (rt_token / runner identity)

| Method | Path | Behavior |
|--------|------|----------|
| POST | `/api/v1/jobs/claim` | Claim the oldest `queued` job for this runtime (legacy poll path). `FOR UPDATE SKIP LOCKED`; sets `status=claimed`, `claim_lease_until=NOW()+30s`, `attempts++`. `204` when nothing to claim |
| POST | `/api/v1/jobs/{id}/ack` | Ownership-checked (`403` if not owner). Status `running\|done\|failed` via the state machine (`409` on invalid transition). On `done`/`failed`: `writeback_status=pending` + async REST write-back. `204` |
| POST | `/api/v1/jobs/{id}/heartbeat` | Extends `claim_lease_until=NOW()+90s`. `204` |
| GET | `/api/v1/jobs/subscribe` | SSE (`text/event-stream`): subscribe to topics (e.g. `runner.<id>.dispatch`); honors `Last-Event-ID` for resume; monotonic ids + heartbeats |
| POST | `/api/v1/jobs/messages/{msgID}/ack` | Acknowledge a delivered bus message (SSE is server→client; ack is out-of-band) |
| POST | `/api/v1/runners/sessions/{id}/result` | Daemon posts a terminal session result (status/summary/error) |
| POST | `/api/v1/runners/sessions/{id}/events` | Daemon republishes a `ChatEvent`; the hub relays it on `session.<id>.control` |
| GET | `/api/v1/agents/{name}/versions/{version}/pull` | Authorized agent pull (runtime + **grant**, `OpPullAgent`); returns artifact-or-reference + `form` |

### Enrollment

| Method | Path | Auth | Behavior |
|--------|------|------|----------|
| POST | `/api/v1/runners/enroll` | registration token | Exchange a one-time token for a durable runner identity (`runner_id` + secret) |

### Management routes (PAT)

| Method | Path | Handler |
|--------|------|---------|
| POST | `/api/v1/runtimes` | `registry.CreateRuntime` — response includes the one-time `Token` |
| GET | `/api/v1/runtimes` | `registry.ListRuntimes` |
| DELETE | `/api/v1/runtimes/{id}` | `registry.DeleteRuntime` |
| POST | `/api/v1/connections` | `connection.CreateConnection` (seals the provider token) |
| GET | `/api/v1/connections` | `connection.ListConnections` |
| GET | `/api/v1/connections/{provider_code}` | `connection.GetConnection` |
| DELETE | `/api/v1/connections/{provider_code}` | `connection.DeleteConnection` |
| POST | `/api/v1/profiles` | `profile.CreateProfile` |
| GET | `/api/v1/profiles` | `profile.ListProfiles` |
| GET | `/api/v1/profiles/{name}` | `profile.GetProfile` |
| PUT | `/api/v1/profiles/{name}` | `profile.UpdateProfile` |
| DELETE | `/api/v1/profiles/{name}` | `profile.DeleteProfile` |
| POST | `/api/v1/agents/{name}/versions` | `catalog.PublishVersion` — publish an **immutable** version (rejects overwriting with different content) |
| GET | `/api/v1/agents` | `catalog.ListAgents` |
| GET | `/api/v1/agents/{name}` | `catalog.ResolveAgent` (`name@version`, missing version → `latest`) → definition metadata |
| POST | `/api/v1/agents/{name}/dispatch` | `catalog.Dispatch` — resolve version, build grant, publish a dispatch (agent ref + grant) to the target runner topic; the runner pulls lazily |
| POST | `/api/v1/runners/registration-tokens` | `registry.IssueRegistrationToken` — one-time enrollment token |
| GET | `/api/v1/channels` | `channel.ListChannels` — active channel bindings (tenant-scoped) |
| GET | `/api/v1/runs/{runID}/artifacts` | list run artifacts *(artifact store is a stub today)* |
| GET | `/api/v1/runs/{runID}/artifacts/{name}` | download a run artifact *(stub)* |

### Interactive session routes (PAT) `[Implemented]`

| Method | Path | Behavior |
|--------|------|----------|
| POST | `/api/v1/sessions` | Create a session (`{agent_name, version?, runner, workspace?}`); owner/tenant from the PAT; dispatches an open-session message to the runner |
| GET | `/api/v1/sessions` | List the caller's sessions (tenant-scoped) |
| GET | `/api/v1/sessions/{id}` | Get a session |
| DELETE | `/api/v1/sessions/{id}` | Close a session |
| POST | `/api/v1/sessions/{id}/messages` | Submit a chat turn (`{role, content}`) → published to `session.<id>.input`; `202` |
| GET | `/api/v1/sessions/{id}/stream` | SSE stream of `token`/`message`/`done`/`error` events from `session.<id>.control` |

### Message bus `[Implemented]`

Broker interface (server-internal, pluggable backend — **in-memory** for tests, **postgres**
for durability; NATS is a stub): `Publish(topic, msg)` · `Subscribe(topics, fromEventID) →
stream` · `Ack(msgID)`. Topics:
- `runner.<id>.dispatch` — hub → runner: one-shot + open-session dispatches.
- `session.<id>.input` — hub → runner: chat turns + control (cancel/close).
- `session.<id>.control` — runner → hub: per-turn `token`/`message`/`done`/`error` events.
- `channel.<provider>.<resource>.<event>` — channel-routed webhook events *(experimental; off by default)*.

`form` (catalog) is type-agnostic: `definition` (manifest: prompt + workflow + declared needs)
or `image` (container reference). Existing `profiles` map onto `definition`-form agents.

## Trigger grammar `[Implemented]`

Parsed in `libs/server/webhook/parse.go` from a ticket comment body:

```
@mework <profile> [workflow] <free instructions>
```

- `@mework` must be at the start of the body, or preceded by a space/newline. No match
  → not a trigger.
- **Word 1** = `profile` (the profile/runtime code).
- **Word 2** = `workflow` **only if** it normalizes to a recognized keyword
  (case-insensitive, trimmed via `NormalizeWorkflow`); otherwise word 2 onward is
  `instructions` and `workflow` is empty.
- Remaining text = `instructions` (capped at 64KB).

Recognized workflow keywords: **`plan`, `cook`, `test`, `review`, `ship`, `journal`**.

## Webhook processing pipeline `[Implemented]`

`POST /webhooks/{provider}` (`libs/server/webhook/handler.go`):

1. `ExtractContainerID` from the raw body → lookup `watched_containers` → `account_id`.
2. Load `provider_connections.webhook_secret`.
3. Verify `X-Mello-Signature` / `X-Mello-Timestamp` / `X-Mello-Delivery-Id`
   (`HMAC-SHA256(secret, timestamp + "." + body)`, ±5-min replay window).
4. `ParseEvent` → `ParseTrigger` on the comment body.
5. Idempotency by delivery id against `jobs.external_event_id`.
6. Actor allowlist check against `account_identities`; self-retrigger guard.
7. Resolve runtime by `profileName == runtimes.code`; cap instructions at 64KB.
8. Snapshot the profile body; decrypt the connection token; fetch ticket
   title/description.
9. `jobs.Enqueue` → `202 Accepted`.

## Data model `[Implemented]`

Embedded goose migrations (`libs/server/platform/store/migrations/`, `000001_init.sql` onward
— tenancy, messages, agent catalog, channel routing, quotas, schedules, …). Entities are
identified by `(provider_code, external_*_id)` so a new provider needs no migration.

| Table | Key columns | Notes |
|-------|-------------|-------|
| `accounts` | `id`, `name`, `created_at` | One per Mello user |
| `provider_connections` | `account_id`, `provider_code`, `webhook_secret`, `mcp_auth_enc`, `config` | Unique `(account_id, provider_code)`; `mcp_auth_enc` holds the AES-sealed provider token |
| `account_identities` | `account_id`, `provider_code`, `external_user_id` | Actor allowlist; unique `(provider_code, external_user_id)` |
| `watched_containers` | `account_id`, `provider_code`, `external_container_id` | Board → account routing; unique `(provider_code, external_container_id)` |
| `runtimes` | `id`, `account_id`, `code`, `label`, `token_lookup`, `last_seen_at`, `status` | `token_lookup` is the HMAC hash (unique); unique `(account_id, code)` |
| `profiles` | `id`, `account_id`, `name`, `body`, `backend_hint`, `harness`, `workflow_config` | Unique `(account_id, name)` |
| `jobs` | `id`, `account_id`, `runtime_id`, `external_task_id`, `external_event_id`, `provider_code`, `external_actor_id`, `status`, `writeback_status`, `task_title`, `task_description`, `profile_body_snapshot`, `workflow`, `instructions`, `claim_lease_until`, `ttl_expires_at`, `attempts`, `result_summary`, `started_at`, `finished_at` | Unique `(provider_code, external_event_id)` for idempotency |

Key indexes: `idx_jobs_claim (runtime_id, status, created_at)`; partial unique
`idx_jobs_one_active_per_runtime (runtime_id) WHERE status IN ('claimed','running')`;
partial `idx_jobs_writeback WHERE writeback_status='pending'`.

**Target tables `[Planned — c0003]`:** `agents` (name) and `agent_versions`
(immutable version, `form`, payload/reference, checksum). Republishing a version with
different content is rejected. `c0002` reframes `jobs` as the durable backing store
behind the bus (keeping the transactional state machine: `running`→`started_at`,
terminal→`finished_at`, terminal states immutable), and removes the long-poll claim and
heartbeat/lease requirements.

## Job state machine `[Implemented]`

Transactional with row locks (`libs/server/orchestrator/state.go`); terminal states
immutable; same-status transition is a no-op.

```
queued   → claimed | failed
claimed  → running | done | failed | queued
running  → done | failed | queued
```

A background **lease sweeper** returns expired-lease jobs to `queued` and drives
pending write-backs.

## Write-back `[Implemented]`

On terminal ack the server sets `writeback_status=pending` and posts the result over
the provider's REST API (Mello `CreateComment`), unsealing the connection credential
only at write time. A **durable outbox** (`pending → processing → done`/`failed`,
sweeper-retried) guarantees exactly-once delivery — no duplicate comment on restart.
The local side never holds provider credentials. See
[runtime-and-sandbox.md](runtime-and-sandbox.md) and
[auth-and-secrets.md](auth-and-secrets.md).

## Client coverage

`libs/client/subscribe/` maps 1:1 onto the API: `Claim`/`Ack`/`Heartbeat`/`Subscribe` (rt_token);
`CreateRuntime`/`ListRuntimes`/`DeleteRuntime`, `Create/Get/List/DeleteConnection`,
`Create/Get/List/Update/DeleteProfile` (PAT). `CreateRuntimeResponse` carries the
one-time `Token`.
