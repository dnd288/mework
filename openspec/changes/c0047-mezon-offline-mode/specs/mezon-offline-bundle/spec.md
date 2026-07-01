# ADDED mezon-offline-bundle

## ADDED Requirements

### Requirement: Offline server is booted by the daemon

When `mework daemon start --offline --with-mezon` is invoked, the daemon SHALL
spawn a `mework-server` subprocess configured with
`DATABASE_URL=sqlite://<workspace>/.mework/data.db`, auto-minted `SERVER_KEY`
and `MEWORK_SECRET_KEY` (32-byte random hex, stored in
`~/.mework/runtime/keys.json` at 0600 perms per the auth-and-secrets
invariants), and `LISTEN_ADDR=127.0.0.1:0` (port chosen by the OS). If the
subprocess exits before `/readyz` returns 200, the daemon SHALL log the failure
with the last 50 lines of the server log, exit non-zero, and `mework daemon
stop` SHALL report nothing to stop.

#### Scenario: Server crashes during boot — orchestrator exits non-zero

- **WHEN** the server subprocess exits before `/readyz` returns 200
- **THEN** the daemon logs the failure with the last 50 lines of the server log, exits with a non-zero status, and `daemon stop` reports nothing to stop

### Requirement: Daemon waits for server readiness

The daemon SHALL poll `GET http://127.0.0.1:<port>/readyz` every 200ms with a
total 10s timeout after starting the server subprocess. On timeout, the daemon
SHALL signal the server `SIGTERM`, wait 5s, send `SIGKILL` if it is still
alive, and exit non-zero.

#### Scenario: `/readyz` exceeds 10s timeout

- **WHEN** the server does not return 200 on `/readyz` within 10s
- **THEN** the daemon sends the server `SIGTERM`, waits 5s, sends `SIGKILL` if it is still alive, and exits non-zero

### Requirement: Daemon enrolls a runner against the offline server

The daemon SHALL exchange a registration token for a runner identity by
issuing `POST /api/v1/runners/registration-tokens` followed by
`POST /api/v1/runners/enroll` against the offline server (matching the
canonical runner-enrollment handshake in `libs/server/registry/`). On success,
the daemon SHALL store the returned `rt_token` at
`~/.mework/runtime/runner.token` (0600) and pass it to spawned children via
the `MEWORK_RT_TOKEN` env var. On any 4xx or 5xx, the daemon SHALL tear down
the server and exit non-zero.

#### Scenario: Enrollment fails

- **WHEN** the runner enroll request fails (4xx or 5xx)
- **THEN** the daemon tears down the server and exits non-zero

### Requirement: Daemon spawns the Mezon worker

The daemon SHALL spawn a `mework-mezon-worker` subprocess with
`MEWORK_SERVER_URL=http://127.0.0.1:<port>`,
`REDIS_URL=""` (miniredis fallback per c0046),
`MEZON_APP_ID` and `MEZON_API_KEY` (from
`~/.mework/provider/mezon/credentials.json` if present, or from env), and
`MEWORK_RT_TOKEN` pointing at `~/.mework/runtime/runner.token`.

#### Scenario: Happy path — full stack boots and reaches steady state

- **GIVEN** `mework daemon start --offline --with-mezon` is invoked with valid Mezon credentials
- **WHEN** the orchestrator executes
- **THEN** within 10s the server is ready, the runner is enrolled, the worker is connected to Mezon, and the daemon reports `offline stack ready (server :<port>, worker pid <pid>)`

### Requirement: Child lifecycle is fenced by the daemon

The daemon SHALL track the PIDs of every spawned child in
`~/.mework/runtime/offline-pids.json` (0600 perms). On `SIGINT/SIGTERM` (or
`daemon stop`), the daemon SHALL signal children in **reverse spawn order**:
worker first, server second. Each signal SHALL be `SIGTERM`; if the child
has not exited within 5s, the daemon SHALL escalate to `SIGKILL`. After all
children have exited, the daemon SHALL remove the pidfile. The pidfile SHALL
be written atomically with `O_EXCL` semantics so two `daemon start --offline`
invocations cannot race; on `O_EXCL` failure the daemon SHALL exit with
`daemon already running`.

#### Scenario: `mework daemon stop` shuts down the full stack

- **GIVEN** an offline stack is running with both server and worker
- **WHEN** the user runs `mework daemon stop`
- **THEN** the worker receives `SIGTERM` first, then the server; both exit within 5s; the pidfile is removed

#### Scenario: Two `daemon start --offline` invocations cannot race

- **WHEN** two `daemon start --offline` invocations attempt to start concurrently
- **THEN** exactly one succeeds (the other exits with "daemon already running"); the pidfile is written atomically with `O_EXCL` semantics

### Requirement: Pure-CLI offline mode is preserved

The existing offline-daemon flow (no server, no Mezon bot) SHALL continue to
work unchanged. `mework daemon start --offline --workspace .` (no
`--with-mezon`) SHALL keep its current in-process session + Unix-socket
behavior. The new `--with-mezon` and `--no-server` flags are strictly additive.

#### Scenario: Existing pure-CLI offline flow is unaffected

- **WHEN** `mework daemon start --offline --workspace .` is invoked without `--with-mezon`
- **THEN** the daemon starts an in-process session without spawning a server or worker; `mework agent send` continues to talk over the Unix socket
