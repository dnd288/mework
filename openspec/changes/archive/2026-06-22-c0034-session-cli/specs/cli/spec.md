## MODIFIED Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list`;
and a `session` group to inspect and drive interactive sessions —
`session list`, `session create`, `session send`, `session attach`, `session close`), and
Additional (`login`; `auth status/logout`; `config show/set`; `provider connect`;
`version`). Read commands SHALL support `--json` output. The poll-oriented
`runtime register` / claim framing is replaced by `runner enroll`.

`runner enroll` SHALL perform a real enrollment handshake: it exchanges the supplied
registration token for a durable runner identity by calling the server enrollment
endpoint, persists that identity locally so the daemon can run unattended, and surfaces a
hub rejection as a command error.

The `session` commands SHALL be a real client of the server session API, authenticated as
the human caller (PAT): `session list` queries the server for the caller's sessions;
`session create` creates a session for a named agent (and runner); `session send` submits a
chat turn to a session; `session attach` streams the session's events until a terminal
event or an idle timeout; `session close` closes a session.

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
