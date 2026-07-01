## Context

Currently, mework agents require a running server hub (Postgres + hub binary) and
a provider (Mello) to be useful. The agent daemon connects to the hub via SSE,
receives dispatches, and executes tasks. This architecture makes it impossible
to run a mework agent locally without server infrastructure.

The offline mode strips all server dependencies and runs a self-contained agent
process that reads its definition from the workspace's `mework.yml`, starts the
backend (e.g., Claude Code), and accepts one-shot tasks from the CLI via a
simple IPC channel (Unix socket or FIFO).

### Existing building blocks

- **`libs/client/runner/workspace_start.go`** — `StartWorkspaceSession` opens a
  workspace-bound session with a `FileDefinitionResolver`. This is the core of
  offline-mode session startup, already proven in tests.
- **`libs/runner/interactive_session.go`** — `Session.Send()` feeds a turn over
  stdin, exactly what `mework run` needs.
- **`libs/shared/config/`** — identity and config persistence, reused as-is.
- **`libs/sandbox/engine/local/`** — local subprocess sandbox runner.

## Goals / Non-Goals

**Goals:**
- Allow `mework start --workspace <dir> --offline` to boot a self-contained agent
- Allow `mework run <instruction>` to submit one-shot tasks to the running agent
- Zero external dependencies: no Postgres, no hub, no provider
- Stream agent output to stdout in real-time
- Use existing `workspace_start.go` + `interactive_session.go` as much as possible
- Graceful shutdown via SIGINT/SIGTERM or `mework stop`

**Non-Goals:**
- Persistent session history across restarts (in-memory only)
- Multi-agent orchestration within a single offline process
- Network-accessible agent API (CLI-only IPC)
- Docker or other sandbox engines in offline mode (local-only)

## Decisions

### Decision 1: Unix socket IPC between `mework run` and offline agent

The offline agent listens on a Unix socket (e.g., `/tmp/mework-offline-<workspace-hash>.sock`)
for JSON-RPC messages. `mework run` connects to this socket, sends the
instruction, and streams the response back.

**Rationale:** A Unix socket is:
- Filesystem-scoped (naturally scoped to the local machine)
- Permission-controlled (0600, only the owning user can connect)
- No port conflicts, no firewall issues
- Works even when the user has no network

**Alternatives considered:**
- **TCP localhost**: Port conflicts, firewall issues, less secure.
- **FIFO (named pipe)**: Simplex only — needs two FIFOs for request/response.
- **File-based polling**: Race conditions, no streaming.
- **Standard signals**: Cannot carry data beyond a signal number.

### Decision 2: Reuse `FileDefinitionResolver` for agent definition

The existing `FileDefinitionResolver` from `libs/runner/session_dispatch.go`
already reads `<dir>/mework.yml`. Offline mode uses it directly — no new
resolver needed.

### Decision 3: Reuse `workspace_start.StartWorkspaceSession` for session lifecycle

The existing `StartWorkspaceSession` function opens a sandbox bound to a
workspace directory, resolving the definition from the workspace's `mework.yml`.
It handles the full lifecycle: sandbox start, stdin feed, output collection.

### Decision 4: In-memory bus for offline mode

The offline agent uses an in-memory bus (`libs/server/bus/memory`) instead of
Postgres or HTTP brokers. The daemon's interactive session already uses this
pattern in its test suite — `session.NewManager(memory.New(), ...)`.

### Decision 5: Single command layout

| Command | Action |
|---------|--------|
| `mework start --workspace <dir> --offline` | Starts the offline agent daemon |
| `mework run <instruction>` | Sends a one-shot task and streams response |
| `mework stop` | Stops the offline agent gracefully |

The `start` and `stop` commands are added as subcommands of a new `start` and
`stop` top-level commands, or aliased under `mework daemon start --offline`.
Given the existing CLI structure, `mework daemon start --offline` is the
natural fit — the daemon already manages agent lifecycle.

## Risks / Trade-offs

**[Risk] Local socket file collision** → Mitigation: derive socket path from an
SHA-256 hash of the workspace path, prefixed with `/tmp/mework-offline-`. The
`start` command unlinks the socket before binding and cleans up on shutdown.

**[Risk] Offline agent orphaned on terminal close** → Mitigation: the agent
writes its PID to `~/.mework/offline.pid`. The `stop` command reads the PID
and sends SIGTERM. A health-check endpoint (via socket health query) confirms
liveness.

**[Risk] Backend process (Claude Code) fails to start** → Mitigation: the agent
validates that the backend binary is executable at startup and prints a clear
error: `backend 'claude' not found in PATH`.

**[Trade-off] No session persistence** → Offline mode is for one-shot tasks,
not long-running persistent sessions. History is printed to stdout and lost
when the process exits. Users who need persistence use the server mode.

## Migration Plan

1. Add `mework daemon start --offline` flag and `--workspace` flag to the
   existing `daemonStartCmd` in `libs/client/cli/daemon.go`
2. Add `mework daemon stop` (already exists) — verify it also kills offline agent
3. Add `mework run <instruction>` as a new top-level command in
   `libs/client/cli/cmd_run.go`
4. Create `libs/client/runner/offline.go` — the offline agent lifecycle (socket
   listen, task dispatch, response streaming)
5. Create `libs/client/runner/offline_client.go` — the `mework run` IPC client
6. Wire everything in `libs/client/cli/registerCommands()`
7. Test: `cd /tmp/test-workspace && mework daemon start --workspace . --offline`
   then `mework run "hello" --wait`

## Open Questions

- Should `mework run` support `--wait` (block until done) by default, or have a
  `--detach` flag for fire-and-forget? **Default: --wait is the default and only
  mode (fire-and-forget makes no sense for one-shot tasks).**
