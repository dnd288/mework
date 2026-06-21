# Authentication & Secrets

> Audience: developers and operators. Covers every credential the system uses, how
> tokens are stored and verified, how provider credentials are sealed, the planned
> grant model, and the required environment variables. Status badges:
> **`[Implemented]`** today; **`[Planned — cNNNN]`** under `openspec/changes/`.
>
> Source of truth: `internal/server/{auth,middleware,token,secret}/`,
> `internal/server/config.go`, `internal/cli/config.go`.

## The credentials at a glance

| Credential | Held by | Guards | Storage |
|-----------|---------|--------|---------|
| **Mello PAT** | the user (CLI) | `/api/v1` management routes | env/config only, masked in `config show`, never via `config set` |
| **`rt_token`** `[Implemented]` | the daemon | `/api/v1/jobs/*` | plaintext once at creation; only an HMAC hash stored server-side |
| **provider webhook secret** | the server (DB) | `/webhooks/{provider}` signature | stored per connection |
| **sealed provider token** | the server (DB) | write-back to the provider | AES-256-GCM at rest, unsealed only at write time |
| **registration token** `[Planned — c0005]` | the operator → runner | one-time runner enrollment | short-lived, single-use |
| **runner identity** `[Planned — c0005]` | the runner | SSE subscribe / ack / pull | durable, `~/.mework/` `0600` |
| **grant** `[Planned — c0004]` | travels with each dispatch | the operations a run may perform | signed/sealed, per-run |

## Two-token authentication `[Implemented]`

The server runs two bearer schemes plus webhook signatures.

### PAT — management routes
`internal/server/auth/pat.go`. A bearer **Mello personal access token** authenticates
`/api/v1` management routes (runtimes, connections, profiles). The middleware calls
Mello `/me`, resolves or creates the `accounts` + `account_identities` rows (provider
`mello`), and caches the account by `SHA-256(token)` for 60s (negative-caches 401s
too). On resolve it asynchronously syncs the user's Mello workspaces/boards into
`watched_containers`.

### rt_token — daemon job routes
`internal/server/middleware/runtime_auth.go`. A bearer runtime token (`mework_rt_…`)
authenticates `/api/v1/jobs/*`. The middleware computes
`ComputeLookup = HMAC-SHA256(token, SERVER_KEY)` and matches `runtimes.token_lookup`,
then async-updates `last_seen_at=NOW(), status='online'` and injects `runtime_id` +
`account_id` into the request context.

**Token properties** (`internal/server/token/`): a recognizable prefix + 256-bit
entropy; returned in plaintext **exactly once** at registration; only the
HMAC-SHA256 lookup hash (keyed by `SERVER_KEY`) is stored. A database leak therefore
yields no usable tokens.

### Webhook signature
`/webhooks/{provider}` is not PAT/rt_token-authed; it is **signature-verified** in the
handler. The Mello adapter (`internal/server/provider/mello/adapter.go`) verifies
`HMAC-SHA256(secret, timestamp + "." + body)` (hex, optional `sha256=` prefix stripped,
`hmac.Equal`) within a ±5-minute replay window. The secret is the per-connection
`provider_connections.webhook_secret`.

### Open
`/healthz` is unauthenticated.

## Credential sealing `[Implemented]`

`internal/server/secret/secret.go`. Provider tokens (used for REST write-back) are
sealed with **AES-256-GCM**: key = `SHA-256(MEWORK_SECRET_KEY)`, a random nonce is
prepended to the ciphertext, hex-encoded, stored in
`provider_connections.mcp_auth_enc`. The token is **unsealed only server-side at
write-back time** (`connectionSvc.GetDecryptedToken`) — the daemon never holds it.

## Runner enrollment `[Planned — c0005]`

Replaces `runtime register` for the agent hub. Modeled on `actions/runner config`:

1. The hub issues **short-lived, single-use registration tokens** plus an exchange
   endpoint.
2. `mework runner enroll --url <hub> --token <registration-token>` exchanges the
   registration token once and **persists a durable runner identity at `~/.mework/`
   with `0600`** perms.
3. Registration tokens are **not** reusable as the long-lived identity; invalid/expired
   tokens are rejected and **no identity is persisted on failure**.

Thereafter the runner authenticates the transport routes (SSE subscribe, ack, pull)
with its durable runner identity.

## The grant model `[Planned — c0004]`

Authentication answers *who*; a **grant** answers *what this run may do*.

- A **small enumerable operation set** plus a grant representation that is scoped,
  explicit, and **least-privilege by default** — no grant for an operation means that
  operation is **denied**.
- Grants are **scoped per run, not per identity** — the same runner can be highly
  privileged for one dispatch and minimal for the next.
- Grants are **integrity-protected (signed/sealed)** via the platform secret/token
  primitives, so a runner cannot widen its own scope (grant-forgery defense).
- The grant **travels with the dispatch** for downstream enforcement.

Enforced in three layers: **hub authorizes → runner enforces locally → sandbox
contains.** An authenticated runner is still denied operations outside its current
dispatch's grant. See [philosophy.md](philosophy.md) and
[runtime-and-sandbox.md](runtime-and-sandbox.md).

## Environment variables

### Server (`internal/server/config.go`)

| Env | Required | Default | Purpose |
|-----|----------|---------|---------|
| `DATABASE_URL` | **yes** | — | Postgres DSN |
| `SERVER_KEY` | **yes** | — | HMAC key for `rt_token` lookup hashing |
| `MEWORK_SECRET_KEY` | **yes** | — | AES-256-GCM key for sealing provider credentials |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address |
| `WEBHOOK_SECRET` | no | — | Loaded but not enforced; per-connection secrets in the DB are used instead |
| `MELLO_BASE_URL` | no | `https://mello.mezon.vn/api/v1` | Mello REST base URL |

The server **fails fast at startup** if any required variable is missing.

### CLI / daemon

| Env | Purpose |
|-----|---------|
| `MEWORK_HOME` | Config root override (default `~/.mework`) |
| `MEWORK_PROFILE` | Local profile selection (isolates state under `~/.mework/profiles/<name>/`) |
| `MEWORK_SERVER_URL` | `mework-server` URL (default `http://localhost:8080`) |
| `MEWORK_DEBUG` | Verbose errors |
| `MELLO_BASE_URL` / `MELLO_API_KEY` / `MELLO_WORKSPACE_ID` | Mello REST resolution (precedence: flag > env > config) |

The PAT (`token`) is env-or-config only (no flag) and is set via `mework login`, not
`config set`.

## File permissions `[Implemented]`

Config, pid, log, and credential files are written `0600`; directories are `0700`.
This applies to `~/.mework/config.json`, `daemon.pid`, `daemon.log`, the planned runner
identity, and per-job work directories.
