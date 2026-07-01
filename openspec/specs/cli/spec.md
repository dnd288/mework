# CLI Specification

## Purpose

Define the `mework` command-line surface and configuration model: managing Mello
boards/tickets/comments, registering runtimes and profiles on the server,
connecting providers, and controlling the daemon. Owned by `cmd/mework` and
`internal/cli`.
## Requirements
### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list`;
`server start` to run the hub in-process; a `session` group to inspect and drive
interactive sessions — `session list`, `session create`, `session send`,
`session attach`, `session close`; and a `sandbox` group to run a local
workspace as a worker — `sandbox start`, `sandbox list`, `sandbox stop`,
`sandbox send`), and Additional (`login`; `auth status/logout`; `config
show/set`; `provider connect`; `version`). Read commands SHALL support
`--json` output.

The poll-oriented `runtime register` / claim framing is replaced by
`runner enroll`.

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
than degrading silently.

#### Scenario: New flags are part of the documented command surface

- **WHEN** `mework <cmd> --help` is invoked for any command gaining a new flag in this change (`mework init` gaining `--provider`, `mework daemon start` gaining `--with-mezon` and `--no-server`)
- **THEN** the help output advertises the new flag with a one-line description, alongside the existing flags

### Requirement: `mework init` accepts `--provider`

`mework init` SHALL additionally accept `--provider <name>` where `<name>`
is one of `mezon` (v1). When set, the command writes a `provider: <name>`
block to `mework.yml` along with a default provider-specific policy. When
unset, the original behavior is preserved (no `provider:` block written).

#### Scenario: `mework init --provider mezon` writes the provider block to mework.yml

- **WHEN** the user runs `mework init --workspace . --agent claude --name mybot --provider mezon`
- **THEN** `mework.yml` contains a `provider: mezon` key with a default echo-policy

#### Scenario: `mework init` without `--provider` keeps old behavior

- **WHEN** the user runs `mework init --workspace . --agent claude --name mybot` (no `--provider`)
- **THEN** `mework.yml` is written without a `provider:` key (preserves prior scaffolding)

### Requirement: `mework daemon start` accepts `--with-mezon` and `--no-server` in `--offline` mode

`mework daemon start` SHALL additionally accept `--with-mezon` and
`--no-server` when used together with `--offline`. See the
`mezon-offline-bundle` capability for the full state machine. When
`--offline --with-mezon` is set, the daemon SHALL delegate to the offline-stack
orchestrator described in `mezon-offline-bundle`; when `--offline` is set
without `--with-mezon` (the default), the existing pure-CLI offline flow runs
unchanged.

#### Scenario: `daemon start --offline --with-mezon` is the documented path for offline Mezon

- **WHEN** the user runs `mework daemon start --offline --with-mezon --workspace <dir>` with valid Mezon credentials
- **THEN** the CLI accepts the flag combination and the offline-stack orchestrator runs

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

### Requirement: Configuration resolution

The system SHALL resolve configuration with precedence **flag > environment >
config file**, storing the config file at `~/.mework/config.json`. The
`--profile <name>` flag (or `MEWORK_PROFILE`) MUST isolate config, daemon state,
pid, and logs under `~/.mework/profiles/<name>/`, and `MEWORK_HOME` MUST override
the root directory.

#### Scenario: Flag overrides env and file

- **WHEN** a value is set in the config file, the environment, and a flag
- **THEN** the flag value is used

#### Scenario: Profile isolation

- **WHEN** a command runs with `--profile work`
- **THEN** it reads and writes state under `~/.mework/profiles/work/` instead of the default location

### Requirement: Credential file safety

The system SHALL persist tokens and config with restrictive permissions
(`0600` for files, `0700` for directories) so local credentials are not
world-readable.

#### Scenario: Token stored with restrictive permissions

- **WHEN** the CLI writes a token to the config
- **THEN** the file is created with `0600` permissions

