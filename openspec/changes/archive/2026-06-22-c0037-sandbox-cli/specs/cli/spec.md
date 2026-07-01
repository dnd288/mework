## MODIFIED Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list`;
`server start` to run the hub in-process; a `session` group to inspect and drive
interactive sessions — `session list`, `session create`, `session send`, `session attach`,
`session close`; and a `sandbox` group to run a local workspace as a worker — `sandbox
start`, `sandbox list`, `sandbox stop`, `sandbox send`), and Additional (`login`;
`auth status/logout`; `config show/set`; `provider connect`; `version`). Read commands
SHALL support `--json` output. The poll-oriented `runtime register` / claim framing is
replaced by `runner enroll`.

`runner enroll` SHALL perform a real enrollment handshake: it exchanges the supplied
registration token for a durable runner identity by calling the server enrollment
endpoint, persists that identity locally so the daemon can run unattended, and surfaces a
hub rejection as a command error.

`server start` SHALL run the hub in-process within the `mework` binary, reading the same
configuration as the standalone server from the environment (database URL and the server/
secret keys, with an optional listen-address override), running database migrations, and
serving the hub with graceful shutdown. It is suitable as the command of a container/
docker-compose service alongside a database service. When the binary is built without an
in-process hub available, `server start` SHALL fail with a clear, actionable error rather
than silently doing nothing.

The `session` commands SHALL be a real client of the server session API, authenticated as
the human caller (PAT): `session list` queries the server for the caller's sessions;
`session create` creates a session for a named agent (and runner); `session send` submits a
chat turn to a session; `session attach` streams the session's events until a terminal
event or an idle timeout; `session close` closes a session.

The `sandbox` commands SHALL turn a local workspace into a server-addressable worker.
`sandbox start -w <dir>` (default the current directory) SHALL require a `mework.yml` in the
workspace, resolve the workspace to an absolute path, target the local enrolled runner
identity, and create a workspace-bound session so the local daemon opens a long-lived
sandbox bound to that directory; it SHALL print the session id and MAY stream events with
`--attach`. `sandbox list`, `sandbox stop <id>`, and `sandbox send <id> <message>` SHALL
manage and message a worker by its session id (the latter equivalent to `session send`).
When the machine is not enrolled, `sandbox start` SHALL fail with guidance to enroll/start
the daemon.

#### Scenario: Start the hub in-process

- **WHEN** an operator runs `mework server start` with the required configuration present in
  the environment
- **THEN** the CLI runs database migrations and serves the hub (honoring an optional
  listen-address override), so the server runs without a separate `mework-server` binary

#### Scenario: Server start without configuration

- **WHEN** an operator runs `mework server start` with required configuration missing
- **THEN** the command exits with a non-zero status and an error naming the missing
  configuration, and does not start a partial server

#### Scenario: Enroll a runner from the CLI

- **WHEN** an operator runs `mework runner enroll --url <hub> --token <registration-token>`
- **THEN** the CLI exchanges the token at the hub for a durable identity, persists the
  identity locally, prints the resulting runner ID, and the runner is ready to receive
  dispatches unattended

#### Scenario: Enrollment rejected by the hub

- **WHEN** an operator runs `mework runner enroll` with an invalid or expired registration
  token
- **THEN** the command exits with a non-zero status and an error explaining the hub
  rejected the token, and no identity is persisted

#### Scenario: Required flags

- **WHEN** an operator runs `mework runner enroll` without `--url` or without `--token`
- **THEN** the command fails with a required-flag error and performs no network call

#### Scenario: Start a workspace as a worker

- **WHEN** an operator runs `mework sandbox start -w .` in a folder containing a `mework.yml`
  on an enrolled machine with a running daemon
- **THEN** the CLI creates a workspace-bound session targeting the local runner, the daemon
  opens a long-lived sandbox bound to that folder, and the CLI prints the session id

#### Scenario: Sandbox start without a workspace config

- **WHEN** an operator runs `mework sandbox start -w <dir>` where `<dir>` has no `mework.yml`
- **THEN** the command exits with an error and creates no session

#### Scenario: Sandbox start when not enrolled

- **WHEN** an operator runs `mework sandbox start` on a machine with no runner identity
- **THEN** the command fails with guidance to enroll and start the daemon, and creates no
  session

#### Scenario: Message a worker by id

- **WHEN** an operator runs `mework sandbox send <id> "<message>"` (or `mework session send
  <id> "<message>"`) for a running worker
- **THEN** the turn is delivered to that worker's sandbox and its events stream back

#### Scenario: Create and inspect a session

- **WHEN** an operator runs `mework session create --agent <name>` and then `mework session
  list`
- **THEN** the CLI creates the session via the server and lists it among the caller's
  sessions

#### Scenario: Send a turn and stream the reply

- **WHEN** an operator runs `mework session attach <id>` in one terminal and `mework session
  send <id> "<message>"` in another
- **THEN** the attached terminal streams the session's `token`/`message` events and a
  terminal `done`/`error` for that turn

#### Scenario: Attach exits on idle

- **WHEN** an attached stream receives no events for the idle timeout
- **THEN** `session attach` exits cleanly rather than blocking indefinitely

#### Scenario: Close a session

- **WHEN** an operator runs `mework session close <id>`
- **THEN** the CLI closes the session via the server

#### Scenario: Machine-readable output

- **WHEN** a user passes `--json` to a list/get command
- **THEN** the command emits JSON suitable for scripting
