---
name: "c0042-orchestrator-mcp"
---

## Why

An AI agent running inside a mework interactive session has no way to delegate work to sub-agents, manage sandboxes, or communicate back to the human through the session channel. This limits mework to single-agent-at-a-time use cases. The mework MCP server bridges this gap — it exposes mework's sandbox lifecycle, session context, and notification capabilities as MCP tools so a Claude Code orchestrator agent can spawn child sandboxes for specialized work, monitor their progress, and report results back to the human.

## What Changes

- New `libs/mcp-server/` Go module: an stdio-based MCP server binary (`mework-mcp`) that registers tools via `github.com/mark3labs/mcp-go` and routes calls to mework's existing infrastructure
- Sandbox lifecycle MCP tools: `spawn_sandbox`, `get_sandbox_status`, `list_child_sandboxes`, `destroy_sandbox`, `wait_for_sandbox`
- Notification MCP tools: `notify_human` (send message to session output), `ask_human` (send question and wait for response via session input topic)
- Session context MCP tools: `get_session_context` (return session identity, owner, tenant, workspace path), `write_artifact` (persist content to session workspace)
- Sandbox settings template generator: produces a `.claude/settings.json` that injects `mework-mcp` as an MCP server entry
- `go.work` updated to include the new `libs/mcp-server/` module
- `github.com/mark3labs/mcp-go` reintroduced as a dependency (was previously removed)

## Capabilities

### New Capabilities

- `orchestrator-mcp`: MCP protocol server that exposes mework sandbox lifecycle, session context, and notification as callable tools for an AI agent orchestrator

### Modified Capabilities

- `sandbox-runtime`: new MCP tool surface for spawning and managing child sandboxes
- `session`: new notification channel (`notify_human` / `ask_human`) over the session bus

## Impact

- Purely additive — no existing API, schema, or behavior changes
- New `libs/mcp-server/` Go module added to the workspace (`go.work`)
- `github.com/mark3labs/mcp-go` added back as a dependency in the new module
- E2E test module (`libs/tests/e2e/`) updated to drop the `mcp-go-removed` assertion, or the new module is excluded from that test's scan
- The MCP server binary is not installed by default — it must be configured in a sandbox's `.claude/settings.json` to be used
