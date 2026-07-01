# Design — c0047-mezon-offline-mode

## Goal recap

Make the README's headline *"Offline mode (single binary, no server)"* honest when
combined with Mezon: a developer runs `mework init --workspace . --agent claude
--name mybot --provider mezon`, then `mework daemon start --offline --with-mezon`,
then messages the bot from Mezon — and the only thing they need installed is the
binary itself. No Docker, no Postgres, no Redis, no `brew install postgres`.

The change holds the c0045 invariant that the standalone worker remains the
production-mode path; this change only adds an offline-mode convenience on top.
The Postgres path is untouched.

## Architecture

### Layers

```
┌─ offline daemon process ──────────────────────────────────────────────┐
│  libs/client/cli/daemon.go (orchestrator)                              │
│    └── libs/client/runner/offline_stack.go                             │
│         ├── spawnServer() ──▶  mework-server                          │
│         ├── waitReady()   ──▶  GET /readyz on the chosen port         │
│         ├── enrollRunner() ──▶ POST /api/v1/runners/registration-tokens│
│         │           then POST /api/v1/runners/enroll (canonical handshake) │
│         ├── spawnWorker() ──▶  mework-mezon-worker (c0046 miniredis)  │
│         └── trackPids()/forwardSignals()/cleanup()                    │
└───────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ spawned mework-server (Postgres OR SQLite per DATABASE_URL scheme) ───┐
│  libs/server/platform/store/db.go (store.NewStore)                     │
│    ├── postgres/  (unchanged)                                          │
│    └── sqlite/    (NEW — modernc.org/sqlite)                           │
└───────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ spawned mework-mezon-worker (c0045 + c0046) ──────────────────────────┐
│  REDIS_URL=""  ──▶ embedded miniredis                                  │
│  MEWORK_SERVER_URL=http://127.0.0.1:<port>                             │
│  MEWORK_RT_TOKEN=<from ~/.mework/runtime/runner.token (0600)>          │
│  MEZON_APP_ID / MEZON_API_KEY (from local provider store or env)       │
│  Inbound: Mezon WS → enqueue job → server claims → daemon session runs │
│  Outbound: poll /api/v1/jobs?status=done → bot.SendMessage()           │
└───────────────────────────────────────────────────────────────────────┘
```

### 1. SQLite driver — `libs/server/platform/store/sqlite/`

Mirrors the existing `postgres/` package 1:1:

| File | Purpose |
|---|---|
| `sqlite.go` (driver) | Wraps `modernc.org/sqlite` (`sql.Open("sqlite", dsn)`); selects SQLite pragmas on connect (`journal_mode=WAL`, `busy_timeout=5s`, `foreign_keys=on`). |
| `migrations/0001.sql` | Initial schema for `jobs`, `runtimes`, `profiles`, `agents`, `sessions`, `audit_log`, `runner_identity`. Mirrors Postgres schema; replaces `UUID` columns with `TEXT` and replaces `JSONB` with `TEXT` columns holding JSON-encoded payloads. |
| `jobs.go` / `runtimes.go` / `profiles.go` / … | Per-table implementations of the `Store` interface. Re-implement the methods so behavior matches the existing Postgres impl; signatures identical. |
| `sqlite_test.go` | DB-backed tests using a temp `:memory:` SQLite DB; required to keep `make test` green with `TEST_DATABASE_URL` unset. |

**On `FOR UPDATE SKIP LOCKED`:** Postgres uses row-level write locks during a
transaction. SQLite serializes writes via a database-level lock; the equivalent
of `SELECT … FOR UPDATE SKIP LOCKED LIMIT 1` is `UPDATE jobs SET status='claimed',
runner_id=?, claimed_at=? WHERE id IN (SELECT id FROM jobs WHERE
status='queued' AND provider_code=? ORDER BY created_at ASC LIMIT 1) RETURNING *`
inside `BEGIN IMMEDIATE`. This works because SQLite's `UPDATE … RETURNING` (3.35+)
yields the row only after the row lock is acquired.

**On `pgcrypto` UUID generation:** SQLite has no UUID type. The driver uses
`uuid.NewString()` from `github.com/google/uuid` and stores UUIDs as `TEXT`.

**On `pgx.TextDecoder`/JSONB:** JSONB columns become `TEXT` columns holding JSON
strings. Go structs in `libs/shared/core` unmarshal with `json.Unmarshal`; the
driver does the encoding boundary.

### 2. Driver selection — `libs/server/platform/store/db.go`

The existing factory `store.NewStore(ctx, cfg.DatabaseURL)` (in
`libs/server/platform/store/db.go`, already invoked by
`apps/mework-server/main.go`) dispatches by `DATABASE_URL` scheme to the
matching driver in `libs/server/platform/store/{postgres,sqlite}/`. The scheme
table is owned by the `sqlite-backend` capability; this change does not
duplicate it. An unknown scheme is a hard fail to protect production
deployments from silently falling back to SQLite when a Postgres misconfig
drops the scheme.

### 3. Daemon orchestrator — `libs/client/runner/offline_stack.go`

The orchestrator (daemon process) spawns **3 children**: one `mework-server`
subprocess, plus one `mework-mezon-worker` subprocess; runner enrollment is an
HTTP exchange against the server, not a separate child process.

State machine:

```
              ┌─────────────────────────────────┐
              │ Daemon start (--offline --with- │
              │ mezon)                          │
              └─────────┬───────────────────────┘
                        │
                        ▼
              ┌─────────────────┐
              │ bootServer      │──fail──▶ exit 1 + log
              └────────┬────────┘
                       ▼
              ┌─────────────────┐
              │ waitReady       │──timeout (10s)──▶ SIGTERM server + exit 1
              └────────┬────────┘
                       ▼
              ┌─────────────────┐
              │ enrollRunner    │──fail──▶ SIGTERM server + exit 1
              └────────┬────────┘
                       ▼
              ┌─────────────────┐
              │ bootWorker      │──fail──▶ SIGTERM server + exit 1
              └────────┬────────┘
                       ▼
              ┌─────────────────┐
              │ trackPids       │
              │ forwardSignals  │◀──SIGINT/SIGTERM
              │ cleanup on exit │
              └─────────────────┘
```

Each step logs at INFO; failures log at ERROR with the failing PID and child
log tail (last 50 lines from `~/.mework/runtime/<child>.log`). The pidfile is
written atomically with `O_EXCL` semantics so two `daemon start` invocations
cannot race.

**Pidfile format** (`~/.mework/runtime/offline-pids.json`, 0600 perms):

```json
{
  "workspace": "/home/dev/my-cowork",
  "started":   "2026-07-01T11:30:00Z",
  "children": [
    {"role": "server", "pid": 81234, "port": 52345, "log": "/home/dev/.mework/runtime/server.log"},
    {"role": "worker", "pid": 81235, "log": "/home/dev/.mework/runtime/worker.log"}
  ]
}
```

`mework daemon stop` reads this file and signals children in reverse order
(worker → server), waits 5s for graceful exit, then SIGKILL if needed.

### 4. Mezon credentials resolution

In offline mode, the worker reads `MEZON_APP_ID` and `MEZON_API_KEY` from:

1. Process env (passed through by the daemon from
   `~/.mework/provider/mezon/credentials.json` if present).
2. **NOT** the server's sealed provider credentials — the server doesn't see the
   bot's secret in offline mode; only the worker does. This matches c0045's
   separation of concerns.

`mework init --provider mezon` writes the credentials path to `mework.yml` so
the user can `mework provider mezon set` (existing command) and have the daemon
automatically read them.

### 5. CLI: `mework init --provider`

```bash
mework init --workspace . --agent claude --name mybot --provider mezon
```

Adds a `provider: mezon` line to `mework.yml`. Sets a default policy
(echo-passthrough) so v1 ships and policy authoring is a follow-up. The
`.claude/skills/mezon/` skill stub is intentionally left to c0048 — this change
ships the *plumbing*; the agent-facing UX guidance is its own change.

## Tests

| Test file | Coverage |
|---|---|
| `sqlite_test.go` | Migrations apply; round-trip for jobs/runtimes/profiles/sessions; `FOR UPDATE SKIP LOCKED` equivalent (concurrent claimers, exactly-one winner); WAL-mode restart-after-writer-crash simulation. |
| `store_open_test.go` | Schema dispatch: `postgres://…` → postgres.New (with skip when no DB), `sqlite://…` → sqlite.New, unsupported scheme → error. |
| `offline_stack_test.go` | Orchestrator state machine driven by a stub `exec.LookPath` interface; fail-fast on server crash, on `/readyz` timeout, on enroll failure. Signal forwarding verified via in-memory pipes. |
| `daemon_offline_test.go` (extended) | New `--with-mezon`, `--no-server` flags don't break the existing pure-CLI flow. |
| `cmd_init_test.go` (extended) | `--provider mezon` writes the expected `mework.yml` block. |

## Migration / rollout

- **No data migration**: SQLite is a fresh database for offline mode only; no
  production data is touched.
- **No CLI compatibility risk**: existing `--offline` users see no behavior change
  unless they pass the new `--with-mezon` flag.
- **Feature flag**: the SQLite driver is not selected unless `DATABASE_URL=sqlite://…`
  is set; the orchestrator only spawns children when `--with-mezon` is set.

## Out of scope / explicit non-goals

- Multi-bot per offline agent (one Mezon bot per offline daemon; multi-bot stays
  on the server-mode `mework-mezon-worker` path).
- Hot-upgrading an offline stack to a remote server (or vice versa).
- SQLite replication / HA (irrelevant for single-user workstation mode).
- Migrating existing Postgres data into SQLite.
- Authoring the `.claude/skills/mezon/` body (left to a follow-up change).
