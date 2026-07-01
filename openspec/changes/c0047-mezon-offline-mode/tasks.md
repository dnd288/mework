# Tasks — c0047-mezon-offline-mode

## Task [1]: SQLite schema migrations  (tags: db, backend, migration)

Create `libs/server/platform/store/sqlite/migrations/0001.sql` mirroring the
Postgres schema (jobs, runtimes, profiles, agents, sessions, audit_log,
runner_identity). Replace `UUID` with `TEXT`, `JSONB` with `TEXT` (JSON-encoded).
Verification: `openspec validate --strict` passes; `migrate` applies to a
fresh `:memory:` DB without errors.

## Task [2]: SQLite Store implementation  (tags: db, backend)

Implement `libs/server/platform/store/sqlite/` matching the existing
`postgres/` package's exported surface. Includes: `sqlite.go` driver
(`modernc.org/sqlite`, WAL mode, busy_timeout, foreign_keys=on); `jobs.go`,
`runtimes.go`, `profiles.go`, `sessions.go`, `audit.go`, `runner.go` — all
methods mirror the existing postgres impl. Use `UPDATE … RETURNING` for
job-claim. JSON columns marshal/unmarshal explicitly. Verification: every
existing test in `libs/server/orchestrator/*_test.go` that depends on
`TEST_DATABASE_URL` is also runnable against SQLite via a new tag-bridged
build (`go test -tags sqlite`).

## Task [3]: SQLite Store conformance tests  (tags: test, db)

`libs/server/platform/store/sqlite/sqlite_test.go` covers: migration apply,
round-trip CRUD for each table, concurrent claimers (N goroutines claim, M<N
jobs, exactly M get `claimed` rows), `BUSY` handling (claim during writer
contention), restart-after-quit (open fresh connection, verify committed data
survives).

## Task [4]: Store NewStore() scheme dispatch  (tags: backend, config)

`libs/server/platform/store/db.go` — the existing `NewStore(ctx, dsn)` factory
routes by URL scheme: `postgres://…` / `postgresql://…` → postgres driver
(existing), `sqlite://…` / `:memory:` / `file:…` → sqlite driver, anything
else → `fmt.Errorf("unsupported …")`. The server bootstrap
(`apps/mework-server/main.go`) continues to call `store.NewStore(ctx,
cfg.DatabaseURL)`; no signature change is required because the dispatch was
already pre-existing and the SQLite driver is added as a new branch.

## Task [5]: Run-server helper  (tags: cli, backend)

`libs/client/runner/offline_stack.go` — `bootServer` constructs a child
`*exec.Cmd` for `mework-server`, sets `DATABASE_URL=sqlite://…/data.db`,
auto-mints `SERVER_KEY` and `MEWORK_SECRET_KEY` (32-byte random hex, stored
in `~/.mework/runtime/keys.json`), `LISTEN_ADDR=127.0.0.1:0`, captures stdout
to `~/.mework/runtime/server.log`. Returns the chosen port (parsed from the
log line `"listening on 127.0.0.1:<port>"`).

## Task [6]: Wait-for-readyz + enroll  (tags: cli, api, auth)

`offline_stack.go` — `waitReady` polls `GET http://127.0.0.1:<port>/readyz`
every 200ms with 10s total timeout. On success, `enrollRunner` performs the
canonical handshake in `libs/server/registry/enroll.go`: POST to
`/api/v1/runners/registration-tokens` to mint a one-shot registration token,
then POST to `/api/v1/runners/enroll` to exchange it for a durable
`rt_token`. The minted `rt_token` is written to
`~/.mework/runtime/runner.token` (0600) and `bootWorker` passes it to
`mework-mezon-worker` via the `MEWORK_RT_TOKEN` env var.

## Task [7]: Run-worker helper  (tags: cli, backend, infra)

`offline_stack.go` — `bootWorker` constructs a child for
`mework-mezon-worker`. Env: `MEWORK_SERVER_URL=http://127.0.0.1:<port>`,
`MEWORK_RT_TOKEN` (read from `~/.mework/runtime/runner.token`),
`REDIS_URL=""` (so the worker falls back to miniredis per c0046),
`MEZON_APP_ID` / `MEZON_API_KEY` from `~/.mework/provider/mezon/credentials.json`
(if present, else error). Stdout → `~/.mework/runtime/worker.log`.

## Task [8]: Stack lifecycle  (tags: cli, infra)

`offline_stack.go` — `trackPids`, `forwardSignals`, `cleanup`. Pidfile
`~/.mework/runtime/offline-pids.json` (0600). SIGINT/SIGTERM cascades to
children in reverse order (worker, then server). `daemon stop` reads the
pidfile and signals in the same order. Stop timeout 5s, then SIGKILL.

## Task [9]: Daemon `--offline --with-mezon` plumbing  (tags: cli)

`libs/client/cli/daemon.go` — new flags `--with-mezon` and `--no-server` on
`daemon start`. When `--offline` is set *and* `--with-mezon`, `runOfflineForeground`
delegates to `offlineStack.Run`. Pure-CLI offline (no `--with-mezon`) is
unchanged.

## Task [10]: `mework init --provider mezon`  (tags: cli, docs)

`libs/client/cli/cmd_init.go` — `--provider [mezon]` writes a `provider: mezon`
block to `mework.yml` with a default echo-policy. Documented in `docs/cli-and-usage.md`.

## Task [11]: README rewrite — Mezon offline as headline  (tags: docs)

`README.md` — Move the Mezon section above the Server-mode section. Drop the
`(server required)` caveat for offline mode; keep it as a footnote on the
Mezon section. Update the screenshot-style block (`mework init`, `mework
daemon start`, `mework agent send`, now-with-mezon follow-up). Reference
`docs/runtime-and-sandbox.md` for the full stack diagram.

## Task [12]: Stack-level integration test  (tags: test, cli, backend)

`libs/client/runner/offline_stack_test.go` — boots the full stack against a
real `mework-server` binary compiled from the workspace (skip if `go build` of
the server fails). Asserts: server `/readyz` returns 200 within 10s; runner
enroll succeeds; worker connects to Mezon (skipped — no Mezon creds in CI);
the orchestrator correctly tears down children on signal. CI marker
`MEWORK_E2E_OFFLINE=1`.

## Task [13]: Provider credential store compatibility  (tags: cli, config)

`mework provider mezon set` and `mework provider mezon show` keep storing
credentials in `~/.mework/provider/mezon/credentials.json` (0600). The new
offline-stack path in Task [7] reads from this location; no schema change.
Add a test asserting the file permissions are 0600.

## Task [14]: Documentation updates  (tags: docs, infra)

`docs/cli-and-usage.md`, `docs/runtime-and-sandbox.md`, `docs/deployment-guide.md`,
`docs/architecture.md` — update to mention the offline-stack path. Note that
SQLite is **offline-only**; production still requires Postgres.
