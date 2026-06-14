# mello

A Go CLI and agent-runtime daemon for [Mello](https://mello.mezon.vn), the kanban tool.

`mework` manages boards/tickets from the command line and runs a local **agent
daemon** that watches your tickets for a trigger keyword (`/run`) in a comment,
executes a local AI CLI (claude / codex / opencode) against the ticket, and
writes the result back as a comment.

Mello has no server-side push, so the daemon **polls** the REST API. Reads use
the Mello REST API directly; write-backs go through the hosted Mello MCP server.

## Install

```bash
make build        # produces ./bin/mework
# or
go install ./cmd/mework
```

## Quick start

```bash
# 1. Authenticate with a Mello personal access token (validated against /me).
mework login --token mello_pat_xxx
# (omit the value to be prompted, keeping the token out of shell history)

# 2. Point the daemon at the hosted Mello MCP endpoint (required for write-back).
mework config set mcp_url https://<your-mello-mcp-endpoint>

# 3. (optional) Set a default workspace for board/ticket/search commands.
mework config set workspace_id <workspace-id>

# 4. Start the agent daemon.
mework daemon start          # background; --foreground to run in-process
mework daemon status
mework daemon logs -f

# 5. Trigger an agent run: comment "/run <instructions>" on any ticket.
#    The daemon posts a start comment, runs the AI CLI, and posts the result.
```

## Commands

| Group | Commands |
|-------|----------|
| Core | `workspace list`, `board list/get`, `ticket list/get/create/move`, `comment list/add`, `search` |
| Runtime | `daemon start/stop/status/restart/logs` |
| Additional | `login`, `auth status/logout`, `config show/set`, `version` |

Most list/get commands accept `--json`. Global flags: `--server-url`,
`--workspace-id`, `--profile`, `--debug`.

## How the trigger works

The daemon polls watched boards every few seconds and scans each ticket's
comments for the trigger keyword. A comment fires a run when:

- its body contains the keyword (default `/run`, configurable), **and**
- it was **not** authored by the daemon's own user (its start/done comments
  never re-trigger it), **and**
- it has not already been handled (tracked per-ticket in `state.json`).

The triggering comment is marked handled *before* the agent runs, so a crash
mid-run will not re-execute it on restart. Ticket content is fed to the AI CLI
over **stdin** (never as a shell argument) to avoid command injection.

## Configuration

Config lives at `~/.mework/config.json` (use `--profile <name>` to isolate
config, daemon state, pid, and logs under `~/.mework/profiles/<name>/`).
Resolution precedence is **flag > environment > config file**.

| Key / Env | Purpose |
|-----------|---------|
| `MELLO_API_KEY` / `token` | Bearer token for REST + MCP |
| `MELLO_BASE_URL` / `base_url` | REST base (default `https://mello.mezon.vn/api/v1`) |
| `mcp_url` | Hosted Mello MCP endpoint (required for the daemon) |
| `MELLO_WORKSPACE_ID` / `workspace_id` | Default workspace |
| `daemon.trigger_keyword` | Trigger keyword (default `/run`) |
| `daemon.done_column_id` | Optional column to move finished tickets to |

See [docs/cli-and-daemon-guide.md](docs/cli-and-daemon-guide.md) for details.

## Development

```bash
make test     # go test ./...
make vet      # go vet ./...
make build    # build with version ldflags
make snapshot # goreleaser cross-compile (requires goreleaser)
```
