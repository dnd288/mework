## Context

A mework interactive session runs a Claude Code agent on the developer's machine. That agent currently has no way to delegate work to sub-agents, check on their progress, or communicate back through the session channel. The orchestrator MCP server (`mework-mcp`) bridges this gap by exposing mework's existing infrastructure as MCP tools.

The orchestrator itself is not a new runtime — it is a Claude Code agent running inside a regular mework interactive session. The MCP server runs as a child process of that agent, communicating over stdio. It connects to the existing mework daemon (or in-memory bus for dev mode) to execute sandbox operations and publish session events.

Key existing interfaces this calls (all in `libs/shared/ports/interfaces.go` or `libs/server/`):

| Interface | Location | Role |
|-----------|----------|------|
| `ports.SandboxDriver` | `libs/shared/ports/interfaces.go:21` | Start/Stop/Destroy sandboxes |
| `ports.Sandbox` | `libs/shared/ports/interfaces.go:33` | Exec/Mount/Signals on a running sandbox |
| `sandbox/runtime.Manager` | `libs/sandbox/runtime/manager.go` | Lifecycle wrapper around a driver |
| `bus.Broker` | `libs/server/bus/bus.go:42` | Publish/Subscribe/Ack session messages |
| `transport.Dispatch` | `libs/shared/transport/agent.go:38` | Canonical dispatch message |
| `session.Manager` | `libs/server/session/session.go` | Create/Get/List/Attach/Close sessions |
| `session.Dispatcher` | `libs/server/session/handlers.go:18` | DispatchSessionToRunner |

## Goals / Non-Goals

**Goals:**
- Build an stdio-based MCP server binary (`mework-mcp`) that registers and handles tools
- Implement sandbox lifecycle tools: spawn, status, list, destroy, wait
- Implement notification tools: notify_human (publish to session output topic), ask_human (publish question, block for response)
- Implement session context tools: get_session_context, write_artifact
- Generate a `.claude/settings.json` template that injects `mework-mcp` as a configured MCP server
- Preserve the stdin-not-argv invariant for all child sandbox prompts
- All tests pass with `-p 1` (serialized, DB-backed test pattern)

**Non-Goals:**
- Not building a standalone orchestrator daemon or runtime — Claude Code IS the orchestrator
- Not adding HTTP transport for MCP — stdio is the standard Claude Code MCP transport
- Not persisting child sandbox state across MCP server restarts (the orchestrator saves state via write_artifact)
- Not modifying existing mework interfaces or behavior — the MCP server only calls them

## Decisions

- **Stdio transport**: Claude Code launches MCP servers as subprocesses over stdin/stdout. This is the standard pattern and avoids port conflicts, auth, and TLS overhead.
- **`github.com/mark3labs/mcp-go` for protocol**: This is the most widely adopted Go MCP library. It was previously a dependency and was removed — adding it back in the new module avoids touching existing modules.
- **In-memory child sandbox registry**: Thread-safe map within the MCP server process. Child sandboxes are ephemeral — if the MCP server restarts they are lost. The orchestrator is responsible for persisting work-in-progress references via `write_artifact`.
- **`libs/mcp-server/` as a new Go module**: Follows the existing `libs/*` layout pattern. Added to `go.work` like all other libs modules. Its `go.mod` imports `mework/libs/shared`, `mework/libs/server`, and `mework/libs/sandbox`.
- **`notify_human` publishes to `session.<id>.output` topic**: Reuses the existing session bus topic naming convention. The session's SSE stream picks up these messages and delivers them to the human client.
- **`ask_human` subscribes to `session.<id>.input`**: Publishes a question to the output topic, then subscribes to the input topic and blocks until a matching response arrives or a timeout fires. This mirrors chat conversation flow.

## Risks / Trade-offs

- **[Ephemeral children]**: Child sandbox state is lost if the MCP server crashes or restarts. Mitigation: the orchestrator writes child references to the session workspace via `write_artifact` before long operations, and can rehydrate on restart.
- **[Dependency reintroduction]**: `github.com/mark3labs/mcp-go` is re-added as a dependency. Mitigation: scoped to the new `libs/mcp-server/` module only — existing modules remain unchanged. The e2e assertion that checks for mcp-go absence must be updated to exclude this module.
- **[Resource sharing]**: The MCP server runs on the host, not inside a sandbox. A runaway MCP server could consume host resources. Mitigation: the sandbox settings template limits the MCP server to reading the sandbox workspace; the orchestrator agent is responsible for the agent's Claude Code invocation.
- **[Blocking ask_human]**: `ask_human` blocks the MCP server's tool handler goroutine until a response arrives or the timeout fires. Mitigation: timeouts are mandatory (default 60s, configurable); the MCP server runs multiple goroutines so other tools remain responsive.

## Migration Plan

Purely additive — no migration needed. The new `libs/mcp-server/` module and binary do not affect existing functionality. No schema changes, no API changes, no configuration changes. The MCP server is opt-in: it only activates when configured in a sandbox's `.claude/settings.json`.
