# Mework CLI Daemon — Tech Stack

Status: approved-pending-plan
Date: 2026-06-12

## Decision summary
Go CLI + daemon mirroring Multica (`~/src/multica/server/cmd/multica`). Daemon = poll-based AI-agent runtime for Mello (kanban). Triggered by a `/run` comment keyword on a ticket. Consumes the Mello MCP server (hosted remote, HTTP/SSE) for write-back ops; polls Mello REST directly for hot-path reads. Go module path = `mework`. Multi-CLI runtime (claude + codex + opencode).

## Core stack

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.23+ | Mirror Multica; single static binary; cross-compile (goreleaser) |
| CLI framework | `github.com/spf13/cobra` | Same as Multica; command groups (core/runtime/additional) |
| MCP client | `github.com/mark3labs/mcp-go` | Mature Go MCP client; connects to hosted Mello MCP over HTTP/SSE |
| HTTP (REST poll) | stdlib `net/http` | Hot-path ticket/comment reads to Mello REST; bearer auth |
| Shell-word parsing | `github.com/mattn/go-shellwords` | Parse AI CLI invocation (as Multica) |
| Daemonize (unix) | `syscall.SysProcAttr{Setsid:true}` | Detach from shell (as Multica) |
| Daemonize (win) | `DETACHED_PROCESS` + `CREATE_BREAKAWAY_FROM_JOB` | Survive console close (as Multica) |
| Config | JSON at `~/.mework/config.json` | Mirror `~/.multica/`; `--profile` isolation |
| Release | goreleaser + Makefile | Versioned archives, self-update (`mework update`) |
| Tests | stdlib `testing` | Mirror Multica's `cmd_*_test.go` pattern |

## Layout (in /Users/mrdnd/src/mework)

```
cmd/mework/            # main.go + cmd_*.go (Cobra commands)
internal/cli/         # REST API client, errors, config read/write, self-update
internal/mcp/         # mark3labs/mcp-go client wrapper (connect hosted Mello MCP over HTTP/SSE)
internal/daemon/      # poll loop, runtime detection, task exec, write-back
internal/agentrun/    # local AI CLI detection (claude/codex/opencode) + process spawn + output capture
go.mod                # module mework
Makefile
.goreleaser.yml
```

## Mello-specific config (env + config.json)
- `MELLO_API_KEY` (bearer; required) — for REST poll + hosted MCP auth header
- `MELLO_BASE_URL` default `https://mello.mezon.vn/api/v1`
- `MELLO_TIMEOUT` default 30s
- `mcp.url` — hosted Mello MCP endpoint (HTTP/SSE). **Required, no default**; daemon errors clearly if unset
- daemon: watched workspaces/boards, trigger keyword (default `/run`), poll interval (default 5s), done-column (optional), AI CLI selection (claude/codex/opencode)

## What we DROP from Multica (server-push specific)
- Gorilla WebSocket wakeup (`internal/daemon/wakeup.go`) — Mello has no push
- Server-side task claim/registration endpoints — replaced by local poll + local state cache
- Cloud runtime fleet (`internal/cloudruntime`) — out of scope

## Resolved decisions
- Go module path: `mework`
- AI CLIs (v1): claude + codex + opencode (detect from PATH/shell, as Multica)
- MCP transport: hosted remote HTTP/SSE; `mcp.url` required config, no default
- MCP auth: `MELLO_API_KEY` bearer passed to hosted endpoint

## Open questions
None blocking. To confirm during impl: exact hosted MCP URL (user supplies), and `mark3labs/mcp-go` HTTP/SSE client API for current version.
