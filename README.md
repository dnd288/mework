# mework

A Go CLI and agent-runtime daemon for [Mello](https://mello.mezon.vn), the kanban tool, integrated with a central webhook-driven server.

`mework` manages boards/tickets from the command line and runs a local **agent
daemon** that polls the central `mework-server` for jobs. The server receives webhook
notifications from providers (like Mello) when a trigger keyword (`/run`) is posted in a comment,
and the local daemon executes the AI CLI (claude / codex / opencode) against the ticket context in
an isolated workspace before returning the result.

## Install

```bash
make build        # produces ./bin/mework and ./bin/mework-server
# or
go install ./cmd/...
```

## Quick start

```bash
# 1. Point the CLI to the central MeWork server.
mework config set server_url http://localhost:8080

# 2. Authenticate with your Mello personal access token.
mework login --token mello_pat_xxx
# (omit the value to be prompted, keeping the token out of shell history)

# 3. Connect a third-party provider account (e.g., mello) to the server.
mework provider connect --token mello_pat_xxx
# (registers the provider credential on the server to enable write-backs)

# 4. Register this local daemon runtime to get a runtime token (rt_token).
mework runtime register --code local-macbook
# (returns a token; configure the runtime token for your daemon as suggested:)
mework config set rt_token mework_rt_xxx

# 5. Create an AI instruction profile on the server.
mework profile create --name default --body path/to/system_prompt.txt --backend claude --harness claude-code

# 6. Start the agent daemon.
mework daemon start          # background; --foreground to run in-process
mework daemon status
mework daemon logs -f

# 7. Trigger an agent run: comment "/run <instructions>" on any ticket.
#    The server receives the webhook, enqueues the job, the daemon claims & executes it, and the server writes back the result.
```

## Commands

| Group | Commands |
|-------|----------|
| Core | `workspace list`, `board list/get`, `ticket list/get/create/move`, `comment list/add`, `search` |
| Runtime | `daemon start/stop/status/restart/logs`, `runtime register/list/revoke`, `profile create/list/update/delete` |
| Additional | `login`, `auth status/logout`, `config show/set`, `provider connect`, `version` |

Most list/get commands accept `--json`. Global flags: `--server-url`,
`--workspace-id`, `--profile`, `--debug`.

## How the trigger works

The central `mework-server` receives webhook events from the connected provider (e.g., Mello).
It scans incoming events/comments for the trigger keyword. A comment fires a job when:

- its body contains the keyword (default `/run`, configurable), **and**
- it was **not** authored by the daemon's own user (to prevent self-retrigger loops), **and**
- it has not already been handled (tracked using unique constraints on `(provider_code, external_event_id)`).

When a job is claimed, the daemon builds the prompt from the canonical job payload and feeds it to the AI CLI over **stdin** (never as a shell argument) inside an isolated workspace.

## Configuration

Config lives at `~/.mework/config.json` (use `--profile <name>` to isolate
config, daemon state, pid, and logs under `~/.mework/profiles/<name>/`).
Resolution precedence is **flag > environment > config file**.

| Key / Env | Purpose |
|-----------|---------|
| `MELLO_API_KEY` / `token` | Bearer token for REST |
| `MELLO_BASE_URL` / `base_url` | REST base (default `https://mello.mezon.vn/api/v1`) |
| `server_url` / `MEWORK_SERVER_URL` | Mework central server endpoint (default `http://localhost:8080`) |
| `rt_token` | Secure runtime registry token for daemon polling and execution |
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
