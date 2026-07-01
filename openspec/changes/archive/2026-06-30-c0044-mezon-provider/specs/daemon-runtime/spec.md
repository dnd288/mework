# Daemon Runtime Specification

## Purpose

Define the local agent daemon: a **stateless** long-poll worker that claims jobs
from `mework-server`, runs an AI coding CLI against the ticket in an isolated
workspace, and reports the result. It holds no provider credentials and persists
no job state of its own. Owned by `internal/daemon` and `internal/agentrun`.

## ADDED Requirements

### Requirement: Daemon can host a Mezon bot proxy

The enrolled daemon SHALL optionally host a Mezon bot listener as a proxy, connecting to Mezon's WebSocket gateway and dispatching received messages to the channel router on the server via the SSE bus. This allows the daemon to serve as the bot endpoint rather than requiring the server to maintain the WebSocket connection.

#### Scenario: Daemon starts Mezon bot

- **WHEN** the daemon enrolls with `mezon_bot: true` in its enrollment specs
- **THEN** the daemon starts a Mezon bot client that connects to Mezon and receives messages

#### Scenario: Daemon routes Mezon messages to server

- **WHEN** the daemon's Mezon bot receives a channel message
- **THEN** the daemon forwards the message to the server via the SSE bus for channel routing

### Requirement: Daemon spec includes mezon-bot capability

The daemon SHALL include `"mezon-bot"` in its enrollment specs when configured to host a Mezon bot. The server SHALL use this spec to select a daemon capable of running the Mezon bot when auto-provisioning channels for Mezon.

#### Scenario: Enrolled daemon advertises mezon-bot

- **WHEN** the daemon enrolls with Mezon bot configuration
- **THEN** it includes `"mezon-bot"` in its `specs` array on enrollment and heartbeat
