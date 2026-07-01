# MODIFIED **cli**

## MODIFIED Requirements

### Requirement: Command surface  *(modified)*

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

### Requirement: `mework init` accepts `--provider`  *(added)*

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

### Requirement: `mework daemon start` accepts `--with-mezon` and `--no-server` in `--offline` mode  *(added)*

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
