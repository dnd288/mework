# CLI & Usage Guide

> Audience: developers using mework from a kanban board or via the offline
> orchestrator. Status badges: **`[Implemented]`** today.

## Two modes

mework has two distinct usage paths:

| | Server mode | Offline mode |
|---|---|---|
| **Requires** | Postgres + Redis + mework-server | Nothing — single binary |
| **Audience** | Teams, production | Dev, testing, single user |
| **Providers** | Mello webhooks, Mezon worker | Mezon bot (standalone) |
| **CLI commands** | `server`, `runner`, `daemon`, `sandbox`, `session`, `profile`, `provider` | `init`, `mezon-worker`, `agent send` |

---

## Offline mode commands

### `mework init`

Scaffold a workspace with orchestrator or worker configuration:

```bash
mework init --workspace . --agent orchestrator
mework init --workspace ./worker --agent worker
```

Creates:
```
mework.yml              agent definition
CLAUDE.md               persona + commands
.claude/settings.json   MCP server config (mework-mcp)
.claude/skills/         session-manager, communicator, planner
.claude/commands/       /sessions, /spawn, /status, /stop
```

### `mework mezon-worker`

Manage the standalone Mezon bot worker process:

```bash
mework mezon-worker start            # background with miniredis
mework mezon-worker status           # check running + configured
mework mezon-worker stop             # graceful shutdown
mework mezon-worker logs -f          # follow logs
```

The worker connects to Mezon via WebSocket (turbo SDK), receives messages,
processes them through the orchestrator agent, and replies back to the channel.
Requires no server, no Postgres, no Redis — embedded miniredis handles state.

### `mework provider mezon`

Configure Mezon bot credentials and manage bot registration:

```bash
mework provider mezon set --app-id <id> --api-key <key>    # store credentials
mework provider mezon show                                  # show (masked)
mework provider mezon bot register --app-id <id> --api-key <key>  # register on server
mework provider mezon bot list                              # list registered bots
mework provider mezon bot remove <id>                       # delete a bot
```

### `mework agent send`

Send a message to a local offline agent or hub unit queue:

```bash
mework agent list                                         # list agents
mework agent send orchestrator "explore the workspace"    # send to local agent
mework agent send orchestrator "list sessions" --wait     # send and wait for response
```

In offline mode, `agent send` routes through the same orchestrator that
handles Mezon messages — both paths reach the same agent session.

---

## Server mode commands

### Provider setup

```bash
mework login --token <mello-pat>                          # authenticate
mework provider connect --provider mello --token <pat>    # store provider token
```

### Runner enrollment

```bash
mework runner enroll --url http://localhost:8080 --token <registration-token>
mework daemon start                                        # start agent daemon
```

### Agent profiles

```bash
mework profile create --name default --body ./prompt.txt --backend claude
mework profile list
```

### Sessions & sandboxes

```bash
mework sandbox start -w .                                  # create workspace session
mework sandbox send <id> "summarize this repo"             # send a turn
mework session attach <id>                                 # stream events
mework sandbox stop <id>                                   # close session
```

---

## Trigger grammar (Server mode webhooks)

Comment on a Mello card:

```
@mework <profile> [workflow] <instructions>
```

- `profile` — the AI profile to use
- `workflow` — optional: `plan`, `cook`, `test`, `review`, `ship`, `journal`
- `instructions` — free-form text

---

## Orchestrator agent (Offline mode)

When the Mezon worker is running, the orchestrator agent handles your requests.

Built-in commands:

| Command | What it does |
|---------|-------------|
| `/sessions` | List all active work sessions |
| `/spawn <task>` | Spawn a child sandbox to work on a task |
| `/status <id>` | Check a session's status |
| `/stop <id>` | Stop and clean up a session |

Skills available to the orchestrator:

- **session-manager** — spawn, track, monitor, clean up sessions
- **communicator** — Mezon-friendly formatting, response templates
- **planner** — break complex requests into parallel tasks

The orchestrator also has MCP tools via `mework-mcp`: `spawn_sandbox`,
`list_child_sandboxes`, `destroy_sandbox`, `notify_human`, `ask_human`,
`get_session_context`, `write_artifact`.
