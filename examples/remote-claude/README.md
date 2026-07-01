# Remote Claude Code — Session-Based Interactive AI

This example turns a local Claude Code install into a **remotely controlled AI agent** with
**three commands**:

```bash
mework server start          # 1. the hub (gateway + registry)
mework daemon start          # 2. the local runner (after login + enroll)
mework sandbox start -w .     # 3. this folder, as a running worker you can message by id
```

The **agent, its daemon, and its sandbox all run on your machine (the runner)** — Claude Code
is never executed on the server. The server only brokers sessions over a message bus, so
**source code and provider credentials stay local**. Once a workspace is running as a worker,
any authorized client messages it **by session id** — from another terminal, machine, or a
pipeline.

## Concept

```
        mework server  (gateway + registry only)
┌─────────────────────────────────────────────────────┐
│  session.<id>.input   (hub → runner: chat turns)     │
│  session.<id>.control (runner → hub: token/done/…)   │
│  • session metadata   • agent/definition catalog     │
│  • message-bus topics  (never spawns a sandbox)       │
└───────┬───────────────────────────────▲──────────────┘
        │ HTTP /api/v1/sessions          │ SSE subscribe
        │ (create / send / stream)       │ (bus push/pull)
        ▼                                │
┌─────────────────┐            ┌─────────┴─────────────────┐
│  Client / CLI   │            │  Runner — YOUR MACHINE     │
│  session send   │            │  ┌──────────┐              │
│  session attach │            │  │  daemon  │ ┌──────────┐ │
│  sandbox start  │            │  │ (runner) │▶│  Claude  │ │
└─────────────────┘            │  │          │ │(sandbox) │ │
                               │  │          │◀│ stdin/out│ │
   Clients drive the worker    │  └──────────┘ └──────────┘ │
   over HTTP+SSE by session id. │  source + creds stay here  │
                               └────────────────────────────┘
```

`mework server` is a **gateway + registry** only: it holds session metadata, the
agent/definition catalog, and the message-bus topics, and routes between clients and the
runner. The **daemon and sandbox run on the runner**; the server never spawns a sandbox or
runs Claude.

## Where things run

| Tier | Runs | Responsibility |
|------|------|----------------|
| **server** (`mework server start` or docker compose) | a host you point clients at | Gateway + registry: session metadata, catalog, bus topics. **Never** runs Claude. |
| **runner** (`mework daemon start`) | **the daemon + sandbox + Claude Code** | Enrolls once, subscribes over SSE, opens the sandbox locally, streams events back. Source + creds live here. |
| **clients** (`mework session …` / `mework sandbox …`) | terminals / pipelines | Start a workspace as a worker, then send turns / stream events by session id. |

## Prerequisites

- Go 1.25+ and the `mework` binary built (`make build`, or `go build ./apps/mework`)
- Postgres — run it yourself, or `docker compose up -d` (see below)
- Claude Code installed (`claude` in PATH) on the runner machine
- A `mework.yml` in your workspace folder (see [`testdata/workspace/mework.yml`](testdata/workspace/mework.yml))

## Quick start — three components

### 1. Start the hub

Locally, in-process:

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework"
export SERVER_KEY="demo-key"
export MEWORK_SECRET_KEY="demo-secret-key-32bytes!!"
mework server start --listen :8080
```

…or bring up the server tier (hub + Postgres) with Docker:

```bash
docker compose up -d          # from examples/remote-claude/
export MEWORK_SERVER_URL=http://localhost:8080
```

(`mework server start` and the `mework-server` compose service serve the same hub.)

### 2. Log in, enroll the runner, start the daemon

On the machine where Claude Code is installed:

```bash
mework login --token <your-mello-pat>

# Issue a one-time registration token (PAT-authed), then enroll this machine:
REG=$(curl -s -X POST "$MEWORK_SERVER_URL/api/v1/runners/registration-tokens" \
        -H "Authorization: Bearer <your-mello-pat>" | jq -r .token)
mework runner enroll --url "$MEWORK_SERVER_URL" --token "$REG"   # writes ~/.mework/identity.json

mework daemon start            # subscribes over SSE; ready to open sandboxes
```

### 3. Turn this folder into a running worker

From a workspace folder containing a `mework.yml`:

```bash
SID=$(mework sandbox start -w . --json | jq -r .id)   # server → dispatch → daemon opens the sandbox bound to .
mework sandbox list                                    # shows SID, agent, status

# stream the worker's events in one terminal:
mework session attach "$SID"

# …message it by id from another terminal (sandbox send == session send):
mework sandbox send "$SID" "summarize this repo and list the entry points"

mework sandbox stop "$SID"     # close the worker (also: mework session close)
```

The turn travels CLI → server (`session.<id>.input`) → daemon → the long-lived sandbox over
**stdin (never argv)**; `token`/`message`/`done` events stream back over `session.<id>.control`
to your attached terminal.

## HTTP API (what the CLI calls)

| Method & path | Auth | Purpose |
|---|---|---|
| `POST /api/v1/sessions` | PAT | Create a session. Body: `{agent_name, version?, runner, workspace?}`. A `workspace` path binds the sandbox to that local dir. |
| `GET /api/v1/sessions` / `GET /api/v1/sessions/{id}` | PAT | List / get sessions (tenant-scoped). |
| `POST /api/v1/sessions/{id}/messages` | PAT | Submit a chat turn: `{role:"user", content}`. |
| `GET /api/v1/sessions/{id}/stream` | PAT | SSE stream of `token`/`message`/`done`/`error` events. |
| `DELETE /api/v1/sessions/{id}` | PAT | Close the session. |
| `POST /api/v1/runners/sessions/{id}/result` · `/events` | runtime (`rt_`) | Daemon-only: report terminal result / republish events. |

## Workspace-bound sessions

A session is **bound to a workspace directory** so the agent reads and writes files in place.
The workspace carries its own definition in a `mework.yml` (plus optional
`.claude/settings.json`). `mework sandbox start -w .` sends the workspace's **absolute path**
on the create request; the daemon resolves the definition from `<dir>/mework.yml` and binds
the sandbox to the directory.

The same fixture drives **two start modes** (both exercised by the Python E2E test, which
runs the real binaries):

- **Local-direct** *(no server, no Postgres)* — you point `mework sandbox start` at a local
  `mework.yml`; the daemon resolves it directly and binds the sandbox. Nothing contacts the
  server.
- **Server** — the workspace path flows through `POST /api/v1/sessions` → dispatch → daemon,
  which resolves `mework.yml` locally and binds the dir. The **agent still runs as a sandbox
  on the runner** — the server never spawns one.

In both modes the turn text is fed over **stdin (never argv)**, the backend runs with its CWD
set to the bound workspace, and produced artifacts persist on disk and are **readable back**.

### `mework.yml`

```yaml
name: workspace-agent
version: 1.0.0
engine: local        # local | docker | cloudflare | custom
backend: claude       # command[0]; the turn arrives on stdin
```

The local engine runs `backend` as `command[0]` with the working directory set to the
workspace. (The Python E2E test rewrites `backend` to the absolute path of
`testdata/stub-backend.sh` so the run is deterministic and needs no real Claude Code.)

### Pack → push → pull

A bound workspace round-trips through the catalog bundle form (exposed as
`mework workspace pack|push|pull`):

- **Pack** — `mework workspace pack` zips the workspace (`mework.yml`,
  `.claude/settings.json`, and ordinary files, preserving nested paths) into a bundle.
- **Pull** — `mework workspace pull` recreates the workspace in a fresh dir with
  identical contents, ready to start a session against.

## Tests

```bash
# Full end-to-end tests — pure Python (stdlib), drives the real binaries + HTTP:

cd /path/to/mework
python3 examples/remote-claude/scripts/e2e.py
```

The E2E test (`scripts/e2e.py`) is a black-box test that builds the `mework` binary,
stands up a hub + mock Mello server, enrolls a runner, starts the daemon, opens a
workspace-bound sandbox backed by the deterministic stub
([`testdata/stub-backend.sh`](testdata/stub-backend.sh)), sends a chat turn, and
asserts the output streams back and the artifact lands on disk.

It replaces the earlier Go in-process tests — Python cannot import Go libraries, so it
tests the exact same flows through the real binary CLI + HTTP API instead:

1. **Build + boot** — `go build ./apps/mework`, start the hub on a free port.
2. **Enrollment** — `login` (PAT auth against a mock Mello), `runner enroll` (one-time
   registration token), `daemon start` (SSE subscription).
3. **Workspace sandbox** — `sandbox start -w <fixture>` opens a session whose daemon
   runs the stub backend with CWD = workspace.
4. **Chat turn** — `session send` feeds the task over stdin; `session attach` streams
   events back.
5. **Assertion** — the stub writes a deterministic artifact into the bound workspace;
   the test checks both the file content and the attach output stream.

The test **skips** (exit 0) if the Go toolchain or Postgres is unavailable, so it's
safe to run in CI without special setup.

## Extending

- **Multi-turn chat** — `sandbox start` opens a long-lived sandbox; keep sending turns by id.
- **File access** — the bound workspace is the agent's working dir; artifacts persist on disk.
- **Multiple workers** — start several workspaces; each is a session addressable by its id.
- **CI/CD** — script `sandbox start --json` + `session send` + `session attach` in a pipeline.
