# Remote Claude Code — Session-Based Interactive AI

This example demonstrates how **mework** turns a local Claude Code installation into a
**remotely controlled AI agent**. Instead of running Claude Code directly in your
terminal, you create a **session** on the mework server, and any authorized client can
send prompts and receive responses — from another terminal, another machine, or an
automated pipeline.

## Concept

```
┌─────────────────────────────────────────────────────┐
│                   mework-server                      │
│                                                      │
│  ┌──────────┐   session.<id>.control (bus topic)     │
│  │ Session  │◄────────────────────────────────────┐  │
│  │ Manager  │                                     │  │
│  │          │  Push(msg) → topic                  │  │
│  │  create  │  Events() ← topic                   │  │
│  │  attach  │                                     │  │
│  │  close   │                                     │  │
│  └──────────┘                                     │  │
│        │                                          │  │
│        ▼                                          │  │
│  ┌──────────┐     SSE subscribe                   │  │
│  │  Worker  │◄────────────────────────────────────┘  │
│  │ (daemon) │                                        │
│  │          │     ┌──────────────┐                   │
│  │          │────▶│  Claude Code │  stdin/stdout     │
│  │          │     │  (sandbox)   │                   │
│  │          │◀────│              │                   │
│  └──────────┘     └──────────────┘                   │
└─────────────────────────────────────────────────────┘
        ▲                                        ▲
        │  HTTP (/api/v1/sessions)               │ SSE
        │                                        │
   ┌─────────┐                             ┌─────────┐
   │ Client A│                             │ Client B│
   │ (remote)│                             │ (remote)│
   └─────────┘                             └─────────┘
```

## What this proves

1. **Claude Code runs as a managed session** — not tied to your terminal
2. **Multiple clients can interact** — push messages, receive events
3. **Session persists across disconnects** — resume from another machine
4. **Same Claude Code experience** — multi-turn chat, file access, tool use

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
| **Sandbox** | Claude Code runs as an isolated subprocess on the worker |
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

### 1. Start mework-server

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework"
export SERVER_KEY="demo-key"
export MEWORK_SECRET_KEY="demo-secret-key-32bytes!"
./bin/mework-server
```

### 2. Enroll a runner

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

### 3. Create a session

```bash
SESSION=$(curl -s -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer your-pat" \
  -d '{"agent_name":"claude-code","runner":"<runner_id>"}' | jq -r '.id')
echo "Session: $SESSION"
```

### 4. Chat with Claude remotely

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
