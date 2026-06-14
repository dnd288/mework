# CLI and Agent Daemon Guide

Operational reference for the `mework` CLI and its agent-runtime daemon.

## Architecture

```
mework CLI ──REST──▶ Mello API (read + CRUD)
   │
   └─ daemon ──poll──▶ Mello REST (find /run comments)
                ──run──▶ local AI CLI (claude/codex/opencode, prompt via stdin)
                ──MCP──▶ hosted Mello MCP (write back start/done comments)
```

- **Reads** (polling, board/ticket/comment fetch) use the REST API directly.
- **Write-backs** (comments, checklist updates) go through the hosted Mello MCP
  server over HTTP/SSE. `mcp_url` must be configured or the daemon will not start.

## Daemon lifecycle

| Command | Behavior |
|---------|----------|
| `mework daemon start` | Re-execs detached in the background (`--foreground` runs in-process). No-op if already running. |
| `mework daemon stop` | Graceful shutdown via the local health port; falls back to SIGTERM. |
| `mework daemon status` | Reports running/stopped, pid, and health port. |
| `mework daemon restart` | Stops (if running) then starts. |
| `mework daemon logs [-f]` | Prints (and optionally follows) the daemon log. |

State lives under the profile directory (default `~/.mework/`):

- `daemon.pid` — running process id (liveness checked via signal 0, so a stale
  file after a crash is not mistaken for a live daemon).
- `daemon.log` — daemon output.
- `state.json` — per-ticket handled comment-id sets (trigger idempotency).
- `work/<ticket-id>/` — isolated working directory per agent run.

The health/shutdown port is derived deterministically from the profile name
(base `19514` + hash), so each profile gets its own port without extra config.

## Trigger semantics

The poll loop, each interval:

1. Resolves the watched boards (configured `watch_board_ids`, else every board
   across accessible workspaces).
2. Lists each board's tickets, then each ticket's comments.
3. Selects comments that (a) contain the keyword, (b) are not authored by the
   daemon's own user, and (c) are not already in the ticket's handled set —
   ordered oldest-first by `created_at`.
4. For each: marks `in_progress` (persisted first), posts a start comment, runs
   the AI CLI with the ticket+comment as a stdin prompt, posts the result as a
   comment, and records the final status (`done`/`failed`).

**Why self-authored comments are skipped:** the daemon writes start/done
comments back to the same ticket. Without the author filter, those comments
(which may echo the keyword) would re-trigger the daemon endlessly.

## AI backends

Detected from `PATH` in preference order: `claude`, `codex`, `opencode`
(override with `daemon.backends`). If none are installed the daemon logs a
warning and skips triggers rather than failing. The prompt is delivered on
stdin, never via argv/shell, so attacker-controllable ticket text cannot inject
commands.

## Rate limiting

REST `429` / `rate_limited` responses are detected and logged with a hint to
lengthen the poll interval (`daemon.poll_interval_seconds`). The loop continues
rather than crashing.

## Profiles

`--profile dev` isolates all of the above (config, pid, log, state, port, work
dir) under `~/.mework/profiles/dev/`, letting you run multiple independent
daemons (e.g. against different Mello servers or workspaces).

## Not yet implemented

- `mework update` self-update: deferred until the project has a published GitHub
  release. The `.goreleaser.yml` + `Makefile` provide the build machinery; the
  update download/verify/swap flow needs the real release repo first.
- Checklist write-back tick: the MCP client supports checklist tools, but the
  daemon does not auto-tick a checklist item because there is no generic
  "agent done" item convention on a Mello board. Wire it via `done_column_id`
  or a future board-convention setting.
