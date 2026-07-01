## 1. MCP server scaffold (TDD)

- [x] 1.1 Create `libs/mcp-server/` Go module with `go.mod`, add `github.com/mark3labs/mcp-go` dependency, add to `go.work`
- [x] 1.2 Implement `ToolRegistry` struct in `handlers.go`: tool name → handler function mapping with `Register(name, schema, handler)` and `HandleCall(name, arguments)`
- [x] 1.3 Implement `main.go`: stdio-based MCP server using `mcp-go` that registers tools from `ToolRegistry` and calls `server.Serve()`
- [x] 1.4 Implement ListTools handler (returns all registered tool names + JSON Schema definitions) and CallTool handler (dispatches to registered handler or returns error for unknown tools)
- [x] 1.5 Write `mcp_server_test.go`: tests for ListTools returns registered tools, CallTool with known/unknown tools, Ping returns pong
- [x] 1.6 `make vet ./libs/mcp-server/...` and `go test ./libs/mcp-server/...` green

## 2. Sandbox lifecycle MCP tools (TDD)

- [x] 2.1 Implement `ChildSandboxRegistry`: thread-safe map of sandbox_id → `ChildSandbox` (status, result channel, created_at, agent_id)
- [x] 2.2 Implement `spawn_sandbox` tool handler: creates `core.RunSpec`, starts sandbox via `Manager.Start()`, registers child, returns `{sandbox_id}` — prompt fed over stdin
- [x] 2.3 Implement `get_sandbox_status` tool handler: looks up sandbox_id, returns `{status, result, created_at}` or error
- [x] 2.4 Implement `list_child_sandboxes` tool handler: returns array of active children
- [x] 2.5 Implement `destroy_sandbox` tool handler: calls `Manager.Stop()` + `Manager.Destroy()`, removes from registry
- [x] 2.6 Implement `wait_for_sandbox` tool handler: blocks on completion channel with configurable timeout
- [x] 2.7 Write `sandbox_test.go`: table-driven tests for all 5 tools using fake sandbox manager
- [x] 2.8 `make vet` / `go test` green

## 3. Notification MCP tools (TDD)

- [x] 3.1 Implement `notify_human` tool handler: publishes message to `session.<id>.output` topic via bus broker
- [x] 3.2 Implement `ask_human` tool handler: publishes question to output topic, subscribes to `session.<id>.input`, blocks for response with timeout
- [x] 3.3 Write `notify_test.go`: tests for notify_human publishes to session output, ask_human publishes and blocks for response, timeout behavior
- [x] 3.4 `make vet` / `go test` green

## 4. Session context MCP tools (TDD)

- [x] 4.1 Implement `get_session_context` tool handler: returns session_id, owner, tenant, workspace_path
- [x] 4.2 Implement `write_artifact` tool handler: writes content to file under session workspace directory, returns effective path
- [x] 4.3 Write `session_test.go`: tests for get_session_context returns identity, write_artifact persists to workspace
- [x] 4.4 `make vet` / `go test` green

## 5. Sandbox settings template (TDD)

- [x] 5.1 Implement `GenerateSandboxSettings()`: produces `.claude/settings.json` with mework-mcp MCP server entry pointing to the binary path and gh MCP server entry
- [x] 5.2 Write `sandbox_settings_test.go`: tests for valid JSON output, correct mework-mcp configuration, gh entry present
- [x] 5.3 `make vet` / `go test` green

## 6. End-to-end integration test (TDD)

- [x] 6.1 Write `orchestrator_e2e_test.go`: creates orchestrator sandbox, spawns child, monitors completion, verifies notify_human sends message to session output
- [x] 6.2 Integration test passes with `TEST_INTEGRATION=true`

## 7. Validation

- [ ] 7.1 Update E2E test assertion (`libs/tests/e2e/`) to exclude the new `libs/mcp-server/` from the `mcp-go-removed` check
- [ ] 7.2 `make build` green (both binaries build)
- [ ] 7.3 `make test` (no DB) green
- [ ] 7.4 `openspec validate c0042-orchestrator-mcp --strict` passes
