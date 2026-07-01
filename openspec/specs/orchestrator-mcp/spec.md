# Orchestrator MCP

## Purpose

This specification defines an stdio-based MCP server binary (`mework-mcp`) that
provides a tool interface for orchestrator agents. The server exposes tools for
sandbox lifecycle management, human notification, session context retrieval, and
settings generation, allowing the orchestrator agent to spawn and manage child
sandboxes, communicate with the human client, and persist data to the session
workspace.

## Requirements

### Requirement: Orchestrator MCP server

The system SHALL provide an stdio-based MCP server binary (`mework-mcp`) that
registers a set of tools and responds to the MCP protocol `tools/list` and
`tools/call` methods. The server SHALL use `github.com/mark3labs/mcp-go` for
the protocol layer and communicate over stdin/stdout (standard Claude Code MCP
transport).

#### Scenario: MCP server starts and responds to ListTools

- **WHEN** the mework-mcp binary is launched with stdio transport
- **THEN** it responds to a `tools/list` JSON-RPC request with the full set of
  registered tool names and their JSON Schema parameter definitions

#### Scenario: Unknown tool returns MethodNotFound

- **WHEN** `tools/call` is invoked with a tool name not in the registry
- **THEN** the server returns a structured error, not a panic or empty result

#### Scenario: Ping returns pong

- **WHEN** a `ping` method call is sent to the MCP server
- **THEN** the server responds with `pong`

### Requirement: Sandbox lifecycle MCP tools

The MCP server SHALL expose five sandbox lifecycle tools: `spawn_sandbox`,
`get_sandbox_status`, `list_child_sandboxes`, `destroy_sandbox`, and
`wait_for_sandbox`. These SHALL call the existing `ports.SandboxDriver` /
`sandbox/runtime.Manager` interfaces and track child sandboxes in an in-memory
registry scoped to the MCP server process.

#### Scenario: Spawn a child sandbox asynchronously

- **WHEN** `spawn_sandbox` is called with a required `prompt` string,
  `agent_id` string, and optional `image`, `timeout_minutes`, `workspace_path`,
  and `env_vars`
- **THEN** a new sandbox starts via the runtime manager's `Start()` method
- **AND** a non-empty `sandbox_id` is returned immediately (before the child
  completes)
- **AND** the prompt is fed to the child over stdin (not argv), preserving the
  stdin-not-argv invariant

#### Scenario: Poll child sandbox status

- **WHEN** `get_sandbox_status` is called with a valid `sandbox_id`
- **THEN** the tool returns the current status (`"running"`, `"done"`, or
  `"failed"`) and, if terminal, the result payload

#### Scenario: Status for unknown sandbox ID returns error

- **WHEN** `get_sandbox_status` is called with a non-existent `sandbox_id`
- **THEN** the tool returns a `not_found` error

#### Scenario: List all child sandboxes

- **WHEN** `list_child_sandboxes` is called
- **THEN** it returns an array of `{ sandbox_id, agent_id, status, created_at }`
  for every active child sandbox managed by this MCP server process

#### Scenario: Destroy a child sandbox

- **WHEN** `destroy_sandbox` is called with a valid `sandbox_id`
- **THEN** the sandbox is stopped via `Manager.Stop()` and `Manager.Destroy()`
- **AND** it is removed from the active child sandbox registry
- **AND** subsequent `get_sandbox_status` calls for that ID return `not_found`

#### Scenario: Wait for sandbox blocks until completion

- **WHEN** `wait_for_sandbox` is called with a valid `sandbox_id` and a
  `timeout_seconds`
- **THEN** the call blocks until the child sandbox reaches a terminal state
  (done/failed) and returns the result
- **AND** if the timeout expires first, a timeout error is returned

### Requirement: Notification MCP tools

The MCP server SHALL expose two notification tools: `notify_human` and
`ask_human`. These SHALL use the session bus topics (`session.<id>.output` and
`session.<id>.input`) to communicate with the human client.

#### Scenario: notify_human sends message to session output

- **WHEN** `notify_human` is called with a `message` string
- **THEN** the MCP server publishes the message on the `session.<id>.output`
  topic via the bus broker
- **AND** the human client receives it through the session's SSE stream

#### Scenario: ask_human sends question and receives response

- **WHEN** `ask_human` is called with a `question` string and optional
  `timeout_seconds`
- **THEN** the question is published on the `session.<id>.output` topic
- **AND** the MCP server subscribes to `session.<id>.input` and blocks until a
  matching response arrives
- **AND** if the timeout expires, a timeout error is returned

### Requirement: Session context MCP tools

The MCP server SHALL expose two session context tools: `get_session_context`
and `write_artifact`. These provide the orchestrator agent with information
about its own execution context and a way to persist data to the session
workspace.

#### Scenario: get_session_context returns session identity and workspace info

- **WHEN** `get_session_context` is called
- **THEN** it returns `session_id`, `owner`, `tenant`, and `workspace_path` (or
  empty strings for unbound sessions)

#### Scenario: write_artifact persists content to the session workspace

- **WHEN** `write_artifact` is called with a `path` (relative within the
  workspace) and `content` string
- **THEN** the file is written under the session's workspace directory and the
  effective absolute path is returned

### Requirement: Sandbox settings template

The MCP server SHALL generate a `.claude/settings.json` template that configures
`mework-mcp` as an MCP server for the orchestrator sandbox's Claude Code agent.

#### Scenario: Generated settings.json includes mework-mcp entry

- **WHEN** `GenerateSandboxSettings` is called
- **THEN** it returns a valid JSON document parsable as a Claude settings object
- **AND** the document includes the `mework-mcp` MCP server configuration
  pointing to the mework-mcp binary path
- **AND** the document includes the `gh` MCP server entry
