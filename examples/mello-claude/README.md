# Mello + Claude Code — Full Integration Example

This example demonstrates the complete flow:
**Mello kanban → webhook → mework-server → job queue → mework daemon → Claude Code sandbox → result posted back**

## Prerequisites

- Go 1.25+ (`go version`)
- Docker (for sandbox isolation)
- A Mello workspace with admin access
- Claude Code CLI installed (`claude` in PATH)

## Architecture

```
┌─────────────┐     webhook      ┌──────────────┐     poll     ┌──────────────────┐
│  Mello Board │  ──────────────▶ │ mework-server │ ◀─────────── │  mework daemon   │
│  (kanban)    │                  │  (hub)        │             │  (local machine) │
│              │ ◀──────────────── │  job queue    │             │                  │
│  @mework dev │   write-back     │  provider     │             │  ┌────────────┐  │
│  <instruct>  │                  │  gateway      │             │  │ sandbox    │  │
└─────────────┘                   └──────────────┘             │  │ claude     │  │
                                                                │  │ code       │  │
                                                                │  └────────────┘  │
                                                                └──────────────────┘
```

## Step 1: Configure

### 1a. Server environment

```bash
# Required
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework"
export SERVER_KEY="your-hmac-key-here"
export MEWORK_SECRET_KEY="your-aes-key-here"

# Mello connection
export MELLO_BASE_URL="https://mello.mezon.vn/api/v1"
export WEBHOOK_SECRET="your-mello-webhook-secret"

# Optional
export LISTEN_ADDR=":8080"
```

### 1b. Mello workspace setup

1. Create a Mello board with columns: **Backlog → In Progress → Review → Done**
2. Generate a **Personal Access Token (PAT)** from your Mello profile settings
3. Note your **workspace ID** and **board ID**

## Step 2: Start the server

```bash
# Run database migrations and start the hub
./bin/mework-server
# Listening on :8080
```

## Step 3: Register runtime and connect provider

### 3a. Login with Mello PAT

```bash
mework login --token mello_pat_xxx
# Token saved to ~/.mework/config.json (0600 permissions)
```

### 3b. Connect Mello provider

```bash
mework provider connect \
  --provider mello \
  --token mello_pat_xxx \
  --webhook-secret my-webhook-secret
# Sealed credential stored in database
```

### 3c. Register a runtime

```bash
mework runtime register --code my-laptop --label "MacBook Pro M4"
# Returns: rt_token = mrt_abc123def456...
# ⚠️ This token is shown only once — save it!
```

## Step 4: Start the daemon

```bash
# Start daemon with the runtime token
mework daemon start --runtime-token mrt_abc123def456...
# Daemon starts polling POST /api/v1/jobs/claim every 5s
# Prompts delivered to Claude Code over stdin
# Workdir: ~/.mework/runs/<job-id>/ (isolated per run)
```

## Step 5: Trigger a job from Mello

On any Mello ticket, comment:

```
@mework dev review this pull request for security issues
```

Or for a full workflow:

```
@mework senior-dev ship implement user authentication with tests
```

## Step 6: Watch the flow

### Server logs
```bash
# mework-server logs
2026/06/21 10:00:01 POST /webhooks/mello 202 (signed by mello)
2026/06/21 10:00:01 Job enqueued: j_001 (queued)
```

### Daemon logs
```bash
# mework daemon logs
2026/06/21 10:00:06 Claimed job j_001
2026/06/21 10:00:06 Running agent in sandbox...
2026/06/21 10:00:06 Detected backend: claude code v1.2.3
2026/06/21 10:00:06 Running in isolated workdir: ~/.mework/runs/j_001/
2026/06/21 10:06:30 Agent finished (exit 0)
2026/06/21 10:06:30 Posting result back to Mello...
2026/06/21 10:06:31 Done — comment posted to Mello ticket
```

### Mello ticket (after completion)
```
🤖 @mework — workflow: review

**Summary:**
Found 3 security issues in the PR:
1. SQL injection risk in user query (line 142)
2. Missing CSRF token on POST /api/delete (line 87)
3. Hardcoded API key in config file (line 5)

**Recommendations:**
- Use parameterized queries instead of string interpolation
- Add `csrf.Protect()` middleware
- Move secrets to environment variables
```

## Example: Local webhook simulation

If you want to test without a real Mello server:

```bash
# 1. Create a fake webhook payload
cat > /tmp/mello-webhook.json << 'JSON'
{
  "event": "comment.created",
  "ticket_id": "TICKET-42",
  "workspace_id": "ws_demo",
  "board_id": "board_demo",
  "comment_body": "@mework dev review this PR for bugs",
  "author_id": "user_123"
}
JSON

# 2. Sign and send (requires webhook secret)
TIMESTAMP=$(date +%s)
BODY=$(cat /tmp/mello-webhook.json)
SIG=$(echo -n "$TIMESTAMP.$BODY" | openssl dgst -sha256 -hmac "my-webhook-secret" | awk '{print $2}')

curl -X POST http://localhost:8080/webhooks/mello \
  -H "Content-Type: application/json" \
  -H "X-Mello-Signature: sha256=$SIG" \
  -H "X-Mello-Timestamp: $TIMESTAMP" \
  -H "X-Mello-Delivery-Id: $(uuidgen)" \
  -d "$BODY"
```

## Configuration files

### `~/.mework/config.json`
```json
{
  "server_url": "http://localhost:8080",
  "default_profile": "dev",
  "profiles": {
    "dev": {
      "backend": "claude",
      "workflow": "review",
      "timeout": "30m"
    },
    "senior-dev": {
      "backend": "claude",
      "workflow": "ship",
      "timeout": "60m"
    }
  }
}
```

### Environment quick-start

```bash
#!/usr/bin/env bash
# save as start-mework.sh

# Config
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework"
export SERVER_KEY="dev-server-key-123"
export MEWORK_SECRET_KEY="dev-secret-key-456"
export MELLO_BASE_URL="https://mello.mezon.vn/api/v1"
export WEBHOOK_SECRET="dev-webhook-secret"

# Start server in background
./bin/mework-server &
SERVER_PID=$!
echo "Server started (PID: $SERVER_PID)"
sleep 2

# Register runtime
RTOKEN=$(mework runtime register --code demo --label "Demo machine" --json | jq -r '.rt_token')
echo "Runtime token: $RTOKEN"

# Start daemon
mework daemon start --runtime-token "$RTOKEN" &
DAEMON_PID=$!
echo "Daemon started (PID: $DAEMON_PID)"

echo ""
echo "✅ Mello + Claude Code integration is live!"
echo "   Comment @mework dev <instructions> on any Mello ticket to trigger a job."
echo ""
echo "   Press Ctrl+C to stop."
wait
```

## Trigger grammar

The full trigger format is:

```
@mework [profile] [workflow] [instructions]
```

| Part | Required | Example |
|------|----------|---------|
| `@mework` | Yes | Trigger keyword |
| `profile` | No | `dev`, `senior-dev` |
| `workflow` | No | `plan`, `review`, `ship`, `test` |
| `instructions` | Yes | `implement user auth` |

Workflow keywords are case-insensitive (normalized to lowercase).

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| Webhook returns 401 | Wrong/missing signature | Check `WEBHOOK_SECRET` matches Mello config |
| Job stays "queued" | No daemon polling | Start daemon with valid `rt_token` |
| Daemon can't claim | Runtime not registered | Re-register and restart daemon |
| Agent fails | Claude Code not installed | Run `which claude` — must be in PATH |
| Timeout | Job runs > 30 min | Increase timeout in profile config |
