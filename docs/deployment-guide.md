# Deployment Guide

> Audience: operators deploying **`mework-server`** (PostgreSQL) in production,
> and users running the **offline/standalone worker** for local development.

## Server mode (production)

Requires PostgreSQL 13+ and optionally Redis for the Mezon turbo engine.

### Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | **yes** | — | PostgreSQL DSN |
| `SERVER_KEY` | **yes** | — | HMAC key for rt_token (≥16 chars) |
| `MEWORK_SECRET_KEY` | **yes** | — | AES-256-GCM key for credential sealing (≥16 chars) |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address |
| `CHANNEL_ROUTING_ENABLED` | no | `false` | Experimental channel routing |
| `REDIS_URL` | no | — | Redis for turbo engine (Mezon multi-bot) |
| `MELLO_BASE_URL` | no | — | Mello API base URL |

### Database setup

Migrations run automatically on startup. Create an empty database:

```bash
docker run --name mework-pg -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=mework -p 5432:5432 -d postgres:16-alpine
```

### Build

```bash
make build          # all binaries
make build-server   # server only
```

### Docker Compose

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: strongpassword123
      POSTGRES_DB: mework
    ports: ["5432:5432"]
    volumes: [mework_db_data:/var/lib/postgresql/data]

  server:
    build: .
    environment:
      DATABASE_URL: postgres://postgres:strongpassword123@postgres:5432/mework?sslmode=disable
      SERVER_KEY: your-hmac-key-min-16-chars
      MEWORK_SECRET_KEY: your-aes-key-min-16-chars
    ports: ["8080:8080"]
    depends_on: [postgres]

volumes:
  mework_db_data:
```

### Health checks

| Path | Behavior |
|------|----------|
| `GET /livez` | Always 200 (liveness, no DB check) |
| `GET /readyz` | DB ping → 200 or 503 |
| `GET /healthz` | Same as /readyz |

---

## Offline mode (single user, zero deps)

The offline mode runs the **`mework-mezon-worker`** binary standalone. It
requires no PostgreSQL, no Redis, and no server. State is in-memory (embedded
miniredis) and ephemeral — lost on restart.

### Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MEZON_APP_ID` | **yes** | — | Mezon application ID |
| `MEZON_API_KEY` | **yes** | — | Mezon API key |
| `MEZON_CONFIG` | no | — | JSON file with multiple bot configs |
| `REDIS_URL` | no | — | When set, uses real Redis (persistent). When empty, uses embedded miniredis (ephemeral, no install) |

When `REDIS_URL` is empty, the worker logs:
```
WARNING: REDIS_URL not set — using embedded in-memory Redis (state lost on restart)
For production, set REDIS_URL=redis://... for persistent state
```

### Setup

```bash
# 1. Build
make build

# 2. Initialize workspace
mework init --workspace . --agent orchestrator

# 3. Configure Mezon bot (json file or env vars)
cat > bots.json << EOF
{"bots":[{"app_id":"your_app_id","api_key":"your_api_key","plan":"pro"}]}
EOF

# 4. Start worker (no server needed)
MEZON_CONFIG=./bots.json bin/mework-mezon-worker
```

The worker auto-initializes the orchestrator workspace with CLAUDE.md,
MCP config, skills, and commands.

### Workspace scaffolding

```bash
mework init --workspace . --agent orchestrator   # orchestrator agent
mework init --workspace . --agent worker          # worker agent
```

Creates `mework.yml`, `CLAUDE.md`, `.claude/settings.json`, `.claude/skills/`,
and `.claude/commands/`.

---

## Backup & recovery

PostgreSQL backup for server mode:

```bash
pg_dump -U postgres -h localhost -d mework > mework_backup_$(date +%Y%m%d).sql
```

The AES-256-GCM-sealed credentials in the dump can only be unsealed with the
same `MEWORK_SECRET_KEY`. Back it up separately from the database.

Offline mode has no durable state — miniredis is in-memory only.
