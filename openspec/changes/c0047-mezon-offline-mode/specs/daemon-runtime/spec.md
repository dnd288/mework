# MODIFIED **daemon-runtime**

## MODIFIED Requirements

### Requirement: Daemon lifecycle management  *(modified)*

The system SHALL provide lifecycle controls — `daemon start` (background, or
`--foreground` in-process), `stop`, `status`, `restart`, and `logs` — with
per-profile pid, log, and work directories so multiple profiles can run
isolated.

#### Scenario: Inspect a running daemon  *(unchanged)*

- **WHEN** a user runs `mework daemon status`
- **THEN** the system reports whether the daemon is running and its health for the active profile

### Requirement: Offline-stack spawn orchestration  *(added)*

When `daemon start` is invoked with `--offline`, the system SHALL additionally
accept:

- `--with-mezon`: orchestrate a 3-process offline stack (server with SQLite,
  runner enrolled against it via the canonical handshake, Mezon worker as a
  child process). See the `mezon-offline-bundle` capability for the full
  state machine.
- `--no-server`: when `--offline` is set without `--with-mezon`, the existing
  pure-CLI offline flow runs unchanged. (`--no-server` is the default; it
  exists for clarity and for scripts that want to make the intent explicit.)

#### Scenario: `--offline --with-mezon` boots an offline stack

- **WHEN** `mework daemon start --offline --with-mezon --workspace <dir>` is invoked with valid Mezon credentials
- **THEN** the daemon spawns the server (SQLite), enrolls a runner against it, spawns the Mezon worker, and reports `offline stack ready (server :<port>, worker pid <pid>)` once the stack is steady

#### Scenario: `--offline` without `--with-mezon` keeps pure-CLI behavior

- **WHEN** `mework daemon start --offline --workspace <dir>` is invoked
- **THEN** the daemon starts an in-process session, registers a Unix socket for `mework agent send`, and no server or worker is spawned

### Requirement: Offline-stack teardown is reverse-order with SIGKILL escalation  *(added)*

The daemon SHALL conform to the **offline-stack child lifecycle contract** as
defined in the `mezon-offline-bundle` capability (reverse-spawn-order
signaling, SIGTERM → 5s → SIGKILL, pidfile removal in
`~/.mework/runtime/offline-pids.json`).

#### Scenario: `mework daemon stop` tears down the offline stack

- **GIVEN** an offline stack is running with both server and worker
- **WHEN** the user runs `mework daemon stop`
- **THEN** the worker is signaled first, then the server; both exit within 5s; the offline-stack pidfile is removed; the daemon confirms with `offline stack stopped`

#### Scenario: Ctrl-C tears down the offline stack gracefully

- **GIVEN** an offline stack is running
- **WHEN** the user sends SIGINT to the daemon
- **THEN** within 5s the worker and server have both exited, the pidfile is removed, and the daemon exits 0
