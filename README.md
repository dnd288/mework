# mework

A **cowork runtime** that connects providers (Mezon, GitHub, Mello) to
AI agents (Claude Code) running on your machine. Source code and credentials
never leave the device.

```
Provider ──→ mework ──→ AI agent (Claude Code, ...)
  (Mezon,           (orchestrator, session manager, MCP tools)
   GitHub,
   Mello)
```

- **Chat with your agent** from Mezon or the CLI
- **Orchestrator** manages sessions, spawns sandboxes, coordinates work
- **Zero infrastructure** — single binary, no databases to install

## Install

### Quick install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/minhlucncc/mework/main/install.sh | sh
```

Installs `mework`, `mework-server`, `mework-mezon-worker`, and `mework-mcp`
to `/usr/local/bin` (or `~/.local/bin`).

## Quick start

**Prerequisites:** [Claude Code](https://claude.ai) (`claude` in PATH)

### Offline mode (single binary, no server)

Run a local agent daemon — no databases, no infrastructure.

```bash
# 1. Install
curl -fsSL https://raw.githubusercontent.com/minhlucncc/mework/main/install.sh | sh

# 2. Scaffold a workspace
mkdir ~/my-cowork && cd ~/my-cowork
mework init --workspace . --agent claude --name mybot

# 3. Start the offline daemon
mework daemon start --offline --workspace .

# 4. Chat with your agent from the CLI
mework agent send mybot "explore the workspace"
mework agent send mybot "spawn a sandbox to list this repo"
```

The workspace comes with CLAUDE.md, MCP tools, skills, and slash commands
(`/sessions`, `/spawn`, `/status`, `/stop`) — everything the orchestrator agent
needs to manage child sandboxes.

### Mezon bot integration (server required)

Connect a Mezon chat bot so you can message your agent from any device.

```bash
# 1. Have the server running (see "Server mode" below)
# 2. Set your Mezon bot credentials
#    Get app_id + api_key at https://mezon.ai/developers/dashboard
mework provider mezon set --app-id YOUR_APP_ID --api-key YOUR_API_KEY

# 3. Register the bot on the server
mework provider mezon bot register

# 4. Start the Mezon worker
mework mezon-worker start

# 5. Chat from Mezon (@your-bot)
```

## Quick start: Server mode (multi-tenant)

The server powers session management, sandbox orchestration, the job queue,
and provider integrations (Mezon, GitHub, etc.).

```bash
# 1. Start Postgres
docker run -d --name mework-pg -p 5432:5432 postgres:16-alpine

# 2. Start the server
DATABASE_URL=postgres://postgres:postgres@localhost:5432/mework \
  SERVER_KEY=your-key-min-16-chars \
  MEWORK_SECRET_KEY=your-key-min-16-chars \
  bin/mework-server

# 3. Enroll a runner and start the daemon
mework runner enroll --url http://localhost:8080
mework daemon start

# 4. Configure an AI profile
mework profile create --name default --backend claude

# 5. Turn your workspace into a sandbox session
mework sandbox start -w .
```

> **Note:** Commands that talk to the server (`sandbox`, `session`, `profile`,
> `provider`) are authenticated via a Personal Access Token (PAT). Set
> `MEWORK_API_KEY` or run `mework login` after enrolling a runner.

## CLI Commands

```                   
Core:     workspace, board, ticket, comment, search
Runtime:  daemon, agent, profile, runner, runtime, sandbox, session, server
Worker:   mezon-worker (start/stop/status/logs)
Setup:    init, login, provider, config, auth
```

| Command | Purpose |
|---------|---------|
| `mework init --workspace . --agent claude --name mybot` | Scaffold a workspace with CLAUDE.md + MCP + skills |
| `mework daemon start --offline --workspace .` | Start the offline agent daemon (no server needed) |
| `mework mezon-worker start` | Start the standalone Mezon bot worker |
| `mework agent send <name> <msg>` | Send a message to a local or hub agent |
| `mework provider mezon set --app-id ... --api-key ...` | Store Mezon bot credentials locally |
| `mework provider mezon bot register` | Register a Mezon bot on the server |
| `mework session create/attach/send` | Interactive session lifecycle |
| `mework sandbox start -w .` | Turn a workspace into a runnable sandbox |

Full reference: **[docs/cli-and-usage.md](docs/cli-and-usage.md)**.

## HTTP API

| Auth | Routes |
|------|--------|
| **Open** | `GET /healthz` · `GET /livez` · `GET /readyz` · `POST /webhooks/{provider}` |
| **Runtime (`rt_`)** | `/api/v1/jobs/*` · `/api/v1/runners/sessions/*` · `/api/v1/agents/*/pull` |
| **PAT** | `/api/v1/{runtimes,connections,profiles,agents,sessions,channels,mezon/bots}` |

Details: **[docs/api-reference.md](docs/api-reference.md)**.

## Orchestrator agent

The orchestrator is a Claude Code agent with:

- **MCP tools**: `spawn_sandbox`, `list_child_sandboxes`, `get_sandbox_status`,
  `destroy_sandbox`, `notify_human`, `ask_human`, `get_session_context`, `write_artifact`
- **Skills**: session management, task planning, Mezon communication
- **Commands**: `/sessions`, `/spawn <task>`, `/status <id>`, `/stop <id>`

The orchestrator manages work by spawning child sandboxes — each sandbox is an
independent Claude Code agent working on a specific task. The user chats with the
orchestrator to coordinate everything.

## Templates

```
templates/workspace/
├── orchestrator/     ← orchestrator agent (chat, session mgmt)
└── worker/           ← worker agent (spawned by orchestrator)
```

`mework init` copies both templates into your workspace as
`.mework/orchestrator/` and `.mework/worker/`. Each includes
`mework.yml`, `CLAUDE.md`, `.claude/settings.json`, `.claude/skills/`,
and `.claude/commands/`.

## Spec-driven development

All non-trivial changes start as specs using OpenSpec:

```
/opsx:explore → /opsx:propose → /opsx:spec → /opsx:apply → /opsx:ship → /opsx:archive
```

Shipped changes live in `openspec/changes/archive/`. Details:
[docs/openspec-workflow.md](docs/openspec-workflow.md).

## Development

### From source

Requires **Go 1.26**. For server mode, also need **PostgreSQL** (`make test-db`).

```bash
git clone https://github.com/minhlucncc/mework.git
cd mework && make build    # → bin/mework, bin/mework-server, bin/mework-mezon-worker, bin/mework-mcp
```

### Commands

```bash
make build    # all binaries
make vet      # go vet
make test     # go test -p 1 (DB tests need TEST_DATABASE_URL)
make test-db  # Docker Postgres
```

## Docs

- [docs/product-overview.md](docs/product-overview.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/api-reference.md](docs/api-reference.md)
- [docs/cli-and-usage.md](docs/cli-and-usage.md)
- [docs/deployment-guide.md](docs/deployment-guide.md)
- [docs/auth-and-secrets.md](docs/auth-and-secrets.md)
- [docs/runtime-and-sandbox.md](docs/runtime-and-sandbox.md)
- [docs/openspec-workflow.md](docs/openspec-workflow.md)
- [docs/engineering-skills.md](docs/engineering-skills.md)
- [examples/remote-claude/](examples/remote-claude/)
