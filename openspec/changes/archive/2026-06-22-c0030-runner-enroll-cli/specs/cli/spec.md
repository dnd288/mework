## MODIFIED Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list`
and `session list` to inspect dispatched agents and active sessions), and
Additional (`login`; `auth status/logout`; `config show/set`; `provider connect`;
`version`). Read commands SHALL support `--json` output. The poll-oriented
`runtime register` / claim framing is replaced by `runner enroll`.

`runner enroll` SHALL perform a real enrollment handshake: it exchanges the supplied
registration token for a durable runner identity by calling the server enrollment
endpoint, persists that identity locally so the daemon can run unattended, and surfaces a
hub rejection as a command error.

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

#### Scenario: Inspect active sessions

- **WHEN** an operator runs `mework session list --json`
- **THEN** the CLI emits the active sessions for the enrolled runner as JSON

#### Scenario: Machine-readable output

- **WHEN** a user passes `--json` to a list/get command
- **THEN** the command emits JSON suitable for scripting
