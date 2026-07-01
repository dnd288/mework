## MODIFIED Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
workspace/board/ticket/comment/search), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list` and
`session list` to inspect dispatched agents and active sessions), and Additional
(`login`, `auth status/logout`, `config show/set`, `provider connect`, `version`).
Read commands SHALL support `--json` output. The poll-oriented `runtime register` /
claim framing is replaced by `runner enroll`.

#### Scenario: Enroll a runner from the CLI

- **WHEN** an operator runs `mework runner enroll --url <hub> --token <registration-token>`
- **THEN** the runner is enrolled with a durable identity and is ready to receive dispatches unattended

#### Scenario: Inspect active sessions

- **WHEN** an operator runs `mework session list --json`
- **THEN** the CLI emits the active sessions for the enrolled runner as JSON

#### Scenario: Machine-readable output

- **WHEN** an operator passes `--json` to a list/get command
- **THEN** the command emits JSON suitable for scripting
