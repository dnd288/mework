# Codebase Summary

Go CLI + agent-runtime daemon for Mello (kanban) and central Mework server. Module path `mework`.
Mirrors the Multica daemon's structure; adapts its push-based runtime to
Mello's poll-only model.

## Layout

```
cmd/mework/            Cobra commands (entry point + command groups)
  main.go             root cmd, persistent flags, version, profile()
  help.go             command registration, config show/set
  client.go           REST client builder + workspace-id resolver
  output.go           table / --json rendering helpers
  cmd_auth.go         login, auth status/logout
  cmd_board.go        workspace + board commands
  cmd_ticket.go       ticket, comment, search commands
  cmd_daemon.go       daemon start/stop/status/restart/logs
  cmd_daemon_unix.go  Setsid detach (build tag !windows)
  cmd_daemon_windows.go  DETACHED_PROCESS detach (build tag windows)
  cmd_version.go      version command

cmd/mework-server/    Mework central server entry point
  main.go             config load, migrations run, HTTP listen, graceful shutdown

internal/cli/         config + path + flag-precedence layer
  config.go           Config struct, Load/Save (JSON, 0600)
  paths.go            ~/.mework paths, profile isolation
  flags.go            FlagOrEnv, Resolve{BaseURL,WorkspaceID,Token}

internal/server/      HTTP server, configuration, router, health handlers
  config.go           env config (DATABASE_URL, LISTEN_ADDR, SERVER_KEY)
  router.go           chi router setup with middlewares (request id, logger, recover)
  health.go           GET /healthz database ping check

internal/store/       database connection pool and goose migrations
  db.go               pgxpool wrapper + stdlib sql database connector
  migrate.go          embedded goose migrations up/down runner
  migrations/         SQL migrations (accounts, account_boards, runtimes, profiles, jobs)

internal/mello/       REST client + entity models
  models.go           User, Workspace, Board, Column, …
  models_ticket.go    Ticket, TicketDetail, Comment, Checklist, …
  client.go           HTTP transport, useV1 base-switch, error parsing
  operations.go       read + write REST methods
  errors.go           APIError + exit-code mapping

internal/mcp/         hosted Mello MCP client (write-back)
  client.go           streamable-HTTP client (mark3labs/mcp-go), bearer auth
  writeback.go        create_comment / checklist tool wrappers

internal/agentrun/    local AI CLI detection + execution
  detect.go           PATH lookup for claude/codex/opencode
  runner.go           spawn with stdin prompt, capture output, timeout

internal/daemon/      poll loop + lifecycle
  run.go              main loop: poll → trigger → handle
  trigger.go          findTriggers (keyword + self-skip + created_at order)
  handler.go          start comment → run AI → done comment
  state.go            per-ticket handled comment-id set (idempotency)
  lifecycle.go        pid read/write/liveness, log file, health port
  health.go           loopback /health + /shutdown server
```

## Data flow

Read/Write (CLI): CLI/daemon → `internal/mello` REST client → Mello API.
Write-back (legacy daemon): daemon → `internal/mcp` → hosted Mello MCP.
Trigger state (legacy daemon): `internal/daemon/state.go` → `~/.mework[/profiles/<p>]/state.json`.

Central Server flow:
1. Mello Comment Webhook → POST `/webhooks/mello` → server verifies signature → enqueues job (durable `jobs` table).
2. Daemon long-poll → GET `/v1/jobs/next` → server claims job with advisory lock + lease.
3. Daemon runs agent (stdin prompt) → posts updates directly to Mezon → ACK/report status.

## Key invariants

- Trigger scan skips comments authored by the daemon's own user id
  (`internal/daemon/trigger.go`) to prevent self-retrigger loops.
- Handled comment ids are a set keyed per ticket and marked before the agent
  runs, so restarts never re-execute a handled trigger.
- AI prompt is passed via stdin, never argv (injection safety).
- Config/secret files are written 0600; profile dirs 0700.

## Test coverage

Unit tests cover: flag precedence, config round-trip + profile isolation, REST
error→exit-code mapping + decode, MCP url-required gate, trigger keyword match +
self-skip + ordering, state idempotency + persistence, runtime detection, runner
stdin/exit handling, pid lifecycle + health-port determinism.

Integration tests cover:
- Server: Config env loader validation, `/healthz` DB ping status codes.
- Store: Embedded SQL migration run (up/down rollback) verifying core tables (accounts, account_boards, runtimes, profiles, jobs) and their respective unique/composite/partial indexes against a real Postgres container database.

