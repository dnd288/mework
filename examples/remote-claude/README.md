# Remote Claude Code — Session-Based Interactive AI

This example demonstrates how **mework** turns a local Claude Code installation into a
**remotely controlled AI agent**. The **agent, its daemon, and its sandbox all run on
your own machine (the runner / client)** — Claude Code is never executed on the server.
You enroll the runner once, then create a **session** through the mework server, and any
authorized client can send prompts and receive responses — from another terminal,
another machine, or an automated pipeline. The server only brokers the session; the
work happens on the client, so source code and provider credentials stay local.

## Concept

```
        mework-server  (gateway + registry only)
┌─────────────────────────────────────────────────────┐
│  ┌──────────┐   session.<id>.control (bus topic)     │
│  │ Session  │   • session metadata                   │
│  │ Manager  │   • agent / definition catalog         │
│  │  create  │   • message-bus topics                 │
│  │  attach  │                                         │
│  │  close   │   (never spawns a sandbox)             │
│  └────┬─────┘                                         │
└───────┼──────────────────────────────────────────────┘
        │  HTTP (/api/v1/sessions)        ▲
        │                                 │ SSE subscribe
        ▼                                 │  (bus push/pull)
┌─────────────────┐            ┌──────────┴───────────────┐
│    Client A     │            │  Runner — CLIENT MACHINE  │
│   (remote UI)   │            │                           │
└─────────────────┘            │  ┌──────────┐             │
┌─────────────────┐            │  │  daemon  │             │
│    Client B     │            │  │ (runner) │             │
│   (remote UI)   │            │  │          │ ┌─────────┐ │
└─────────────────┘            │  │          │▶│ Claude  │ │
                               │  │          │ │(sandbox)│ │
   Clients drive the agent     │  │          │◀│ stdin/  │ │
   over HTTP+SSE; they never   │  └──────────┘ │ stdout  │ │
   touch the runner directly.  │               └─────────┘ │
                               │  source + creds stay here │
                               └───────────────────────────┘
```

The **daemon and the sandbox run on the client's machine (the runner)**, never on
the server. `mework-server` is a **gateway + registry** only: it holds session
metadata, the agent/definition catalog, and the message-bus topics, and routes
between remote clients and the runner. It never spawns a sandbox or executes an
agent, so source code and provider credentials stay on the runner.

## What this proves

1. **Claude Code runs as a managed session** — not tied to your terminal
2. **The agent, daemon, and sandbox all run on the client** — never on the server; source + credentials stay local
3. **Multiple clients can interact** — push messages, receive events
4. **Session persists across disconnects** — resume from another machine
5. **Same Claude Code experience** — multi-turn chat, file access, tool use

## Where things run

| Tier | Runs | Responsibility |
|------|------|----------------|
| **mework-server** | a host you point clients at | Gateway + registry only: session metadata, agent/definition catalog, message-bus topics. **Never** spawns a sandbox or runs Claude. |
| **Runner (client machine)** | **the daemon + sandbox + Claude Code (agent)** | Enrolls once, subscribes over SSE, runs the agent locally in a sandbox, streams events back. Source + credentials live here. |
| **Remote clients** | terminals / UIs / pipelines | Create/attach/push/close over HTTP+SSE; drive the agent without ever touching the runner directly. |

## Prerequisites

- Go 1.25+
- Postgres running (for mework-server)
- Claude Code installed (`claude` in PATH)
- mework binaries built (`make build` or `go build ./...`)

## How it works

### Architecture

The mework session system provides:

| Concept | Implementation |
|---------|----------------|
| **Session** | A tracked conversation with lifecycle (create → attach → close) |
| **Control channel** | Bus topic `session.<id>.control` — push messages to the agent |
| **SSE stream** | Subscriber receives events from the session in real-time |
| **Sandbox** | Claude Code runs as an isolated subprocess on the runner (the client's machine), never on the server |
| **Conversation** | Multi-turn chat with history, streaming tokens, cancel |

### API Flow

```bash
# 1. Create a session (returns session ID)
POST /api/v1/sessions
{"agent_name": "claude-code", "runner": "<runner_id>"}

# 2. Attach to the session (get SSE stream URL)
GET /api/v1/sessions/{id}/attach

# 3. Push a message to the agent
POST /api/v1/sessions/{id}/push
{"content": "Review the code in /workspace for bugs"}

# 4. Receive streaming response via SSE
#    Events: token | message | done | error

# 5. Close the session when done
DELETE /api/v1/sessions/{id}
```

## Running the example

### 1. Start mework-server *(on the server host)*

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework"
export SERVER_KEY="demo-key"
export MEWORK_SECRET_KEY="demo-secret-key-32bytes!"
./bin/mework-server
```

### 2. Enroll a runner *(on the client machine where Claude Code is installed)*

This is the machine that will run the daemon, the sandbox, and Claude Code itself.

```bash
# Issue registration token (needs PAT auth)
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/runners/registration-tokens \
  -H "Authorization: Bearer your-pat" \
  -d '{"tenant_id": "00000000-0000-0000-0000-000000000001"}' | jq -r '.token')

# Enroll runner with specs
curl -s -X POST http://localhost:8080/api/v1/runners/enroll \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"code":"remote-agent","label":"Claude Code Runner","specs":["claude-code"]}'
```

### 3. Create a session *(from any remote client)*

```bash
SESSION=$(curl -s -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer your-pat" \
  -d '{"agent_name":"claude-code","runner":"<runner_id>"}' | jq -r '.id')
echo "Session: $SESSION"
```

### 4. Chat with Claude remotely *(from any remote client)*

The prompt is routed by the server to the runner, where Claude executes in its sandbox;
responses stream back over SSE.

```bash
# Push a message
curl -s -X POST "http://localhost:8080/api/v1/sessions/$SESSION/push" \
  -H "Content-Type: application/json" \
  -d '{"content":"Write a simple Go HTTP server"}'

# Attach and stream the response via SSE
curl -s -N "http://localhost:8080/api/v1/sessions/$SESSION/attach"
```

## Standalone test

The Go test in this directory demonstrates the full flow:

```bash
cd examples/remote-claude
go test -v -count=1 -run TestRemoteClaude
```

This test:
1. Detects Claude Code on the local machine
2. Creates a session through the sandbox engine
3. Sends a prompt and captures the AI response
4. Verifies Claude Code was invoked correctly
5. Shows the output

## Extending

The same pattern works for:
- **Chat mode**: Start a long-lived session with conversation history
- **File access**: Mount workspace directories into the sandbox
- **Tool use**: Register MCP tools that the remote Claude can invoke
- **Multi-agent**: Run different Claude instances for different tasks
- **CI/CD**: Trigger Claude from pipelines, capture results
