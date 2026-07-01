---
name: "c0047-mezon-offline-mode"
---

# c0047-mezon-offline-mode

## Why

The README advertises "**Offline mode (single binary, no server)**" but the Mezon
bot integration is gated behind server mode. A user who runs `mework init --provider
mezon` gets a workspace expecting Mezon to work offline, but the Mezon bot currently
ships as `mework-mezon-worker`, which calls `POST /api/v1/jobs/enqueue` and
`GET /api/v1/jobs?status=done` on a running mework-server — there is no server to
call without spinning up Postgres + Redis + Docker. c0046 made the worker self-contained
(miniredis fallback), but the **server dependency and Postgres requirement remain**.
This change closes that gap so that `mework daemon start --offline --with-mezon` is
the only command a developer needs to message their bot from Mezon.

The user-locked decisions (captured during /opsx:explore):

- **Backend driver**: `modernc.org/sqlite` (pure Go, no cgo, cross-compiles).
- **Worker auth**: the daemon (running first) enrolls the runner against the
  offline server using the canonical handshake in `libs/server/registry/`:
  `POST /api/v1/runners/registration-tokens` (mint a one-shot registration
  token) followed by `POST /api/v1/runners/enroll` (exchange for a durable
  `rt_token`). The resulting plaintext `rt_token` is written to
  `~/.mework/runtime/runner.token` (0600) and the worker is spawned with
  `MEWORK_RT_TOKEN` pointing at it. This matches the `auth-and-secrets`
  invariant ("plaintext `rt_token` is returned once and only its HMAC lookup
  hash is stored").
- **Process birth order**: the daemon spawns server + worker as children, in that
  order, and waits for `/readyz` between them.

## What Changes

- **New capability `sqlite-backend`**: the server stores its job queue, runtimes,
  profiles, and session metadata in SQLite (via `modernc.org/sqlite`) when
  `DATABASE_URL=sqlite://…`. A new `libs/server/platform/store/sqlite/` driver
  implements the existing `Store` interface. The existing
  `store.NewStore(ctx, cfg.DatabaseURL)` factory in
  `libs/server/platform/store/db.go` performs scheme dispatch (see
  `sqlite-backend` capability for the dispatch contract).
- **New capability `mezon-offline-bundle`**: `mework daemon start --offline` is
  extended with `--with-mezon` and (when set) `--no-server` and orchestrates the
  full offline stack:

  1. Spawn `mework-server` with `DATABASE_URL=sqlite://<workspace>/.mework/data.db`,
     `SERVER_KEY=<auto>`, `MEWORK_SECRET_KEY=<auto>`, `LISTEN_ADDR=127.0.0.1:0`
     so the server picks a free port.
  2. Wait for `GET /readyz` (with 10s timeout) before proceeding.
  3. Run the canonical runner-enrollment handshake against the server:
     `POST /api/v1/runners/registration-tokens` then
     `POST /api/v1/runners/enroll`, persisting the returned `rt_token` to
     `~/.mework/runtime/runner.token` (0600).
  4. Spawn `mework-mezon-worker` with `MEWORK_SERVER_URL=http://127.0.0.1:<port>`,
     `MEWORK_RT_TOKEN=<rt_token>`, and `MEZON_APP_ID` / `MEZON_API_KEY` (read
     from local provider store or env). The worker falls back to miniredis as
     in c0046.
  5. Track child PIDs under `~/.mework/runtime/offline-pids.json` so
     `mework daemon stop` shuts down the whole stack in reverse order.
  6. Forward `SIGINT/SIGTERM` to all children; clean up PID file on exit.

- **Modified `daemon-runtime`**: the daemon's `--offline` mode gains the
  spawn-with-mezon orchestration. Existing pure-CLI offline flow (no Mezon, no
  server) is preserved by `--no-server` + omitting `--with-mezon`.

- **Modified `cli`**: a new `mework init --provider mezon` flag scaffolds the
  workspace with `mework.yml` policy `provider: mezon` and a `.claude/skills/mezon/`
  skill stub. (The full skill body is left to a follow-up; v1 ships the scaffolding
  + docs so `mework daemon start --offline --with-mezon` Just Works.)

### Breaking changes

- **None for remote-server deployments.** `DATABASE_URL=postgres://…` continues to
  work; `mework server start` without the offline flag behaves exactly as today.
- **Mild breaking for `--offline` semantics**: an existing user running
  `mework daemon start --offline --workspace .` (no server, no Mezon, pure CLI agent)
  is unaffected; the new `--with-mezon` flag is opt-in. Anyone whose script depends
  on a free port or a missing pidfile under `~/.mework/runtime/` should review
  the new layout.

## Capabilities

### New Capabilities
- `sqlite-backend` — server-storage driver for SQLite (offline / single-process mode)
- `mezon-offline-bundle` — daemon-orchestrated 3-process offline stack (daemon, server, worker)

### Modified Capabilities
- `daemon-runtime` — `--offline` becomes an orchestrator when `--with-mezon` is set
- `cli` — adds `--provider` flag to `mework init` for Mezon-aware scaffolding

## Impact

- **New package**: `libs/server/platform/store/sqlite/` implementing the existing
  `Store` interface; mirrors the existing `postgres/` package in shape.
- **New package**: `libs/client/runner/offline_stack.go` — orchestrates
  server-spawn → enroll → worker-spawn lifecycle.
- **Modified package**: `libs/client/cli/daemon.go` — new flags `--with-mezon`,
  `--no-server`; `runOfflineForeground` delegates to the new orchestrator.
- **Modified package**: `libs/client/cli/cmd_init.go` — adds `--provider [mezon]`.
- **Modified package**: `libs/server/platform/store/db.go` — `store.NewStore`
  gains a SQLite branch that delegates to the new `sqlite/` driver; the
  existing Postgres branch is unchanged.
- **New migration**: `libs/server/platform/store/sqlite/migrations/0001.sql` —
  initial schema (jobs, runtimes, profiles, sessions, audit, runner-identity).
- **Modified docs**: `README.md` (move Mezon section above server-mode section),
  `docs/cli-and-usage.md`, `docs/runtime-and-sandbox.md`,
  `docs/deployment-guide.md` (note SQLite path is offline-only).

## Assumptions

- A single-mezon-bot per offline agent is acceptable for v1 (multi-bot is a
  follow-up; c0045's standalone worker keeps the multi-bot path when paired with
  a remote server).
- SQLite is single-writer; we use `BEGIN IMMEDIATE` for job-claim transactions and
  rely on SQLite's per-connection serialization. This is sufficient because only
  one server process writes to the file at a time.
- The `modernc.org/sqlite` driver's pure-Go build keeps mework's
  `curl | sh` installer working unchanged (no gcc dependency).
- Auto-minting `SERVER_KEY` and `MEWORK_SECRET_KEY` for offline mode stores them
  in `~/.mework/runtime/keys.json` (0600 perms per CLAUDE.md invariants); the
  installer guidance recommends the user supply their own for any deployment
  beyond the local workstation.
