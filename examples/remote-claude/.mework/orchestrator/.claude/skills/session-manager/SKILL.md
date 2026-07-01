---
name: session-manager
description: How to spawn, track, monitor, and clean up work sessions using the mework-mcp MCP tools. Use when the user asks you to do work, wants status updates, or needs to manage running sessions.
---

# Session Manager

Orchestrate work by delegating to child sandbox sessions. Each session is an
independent Claude Code agent working on a specific task.

## Tools

- `spawn_sandbox` — creates a new child sandbox to work on a task
- `list_child_sandboxes` — lists all active sessions
- `get_sandbox_status` — checks a specific session's status
- `wait_for_sandbox` — blocks until a session finishes, returns its output
- `destroy_sandbox` — stops and removes a session

## Workflow

### 1. Task arrives

When the user asks you to do something:

1. **Assess** — is this a single task or multiple sub-tasks?
2. **Plan** — for complex work, propose a plan first
3. **Spawn** — call `spawn_sandbox` with:
   - `agent_id`: a short descriptive name (e.g. `"explorer"`, `"api-builder"`)
   - `prompt`: clear, specific instructions for the child agent
   - `workspace_path`: the workspace path for the child to work in

### 2. Track sessions

Keep a mental list of running sessions. When the user asks "what's running?":

1. Call `list_child_sandboxes`
2. Summarize: session names, how long they've been running
3. Highlight any that have finished

### 3. Check status

When the user asks about a specific session:

1. Call `get_sandbox_status` with the sandbox ID
2. Report: running / done / failed, and key output if available

### 4. Wait and report

When you spawn a quick task, use `wait_for_sandbox` to block and get results:

1. Call `wait_for_sandbox` with the sandbox ID and a timeout
2. When it returns, report the output to the user
3. Then call `destroy_sandbox` to clean up

### 5. Clean up

After a session finishes and you've reported the results:

1. Call `destroy_sandbox` to free resources
2. Tell the user the session is cleaned up

## Session naming

Use descriptive, short names for `agent_id`:
- `"explorer"` — exploring a codebase or directory
- `"builder"` — building something
- `"researcher"` — research task
- `"debugger"` — debugging something
- `"reviewer"` — reviewing code
