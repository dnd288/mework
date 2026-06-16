# CLI and Agent Daemon Guide

Operational reference for the `mework` CLI and its agent-runtime daemon.

## Architecture

```
External Task System (e.g. Mello)
      │                               ▲
      │ webhook                       │ write-back (REST API)
      ▼                               │
┌──────────────────────────────────────────────┐
│                MeWork Server                 │
│  - Inbound Adapter                           │
│  - PostgreSQL Job Queue                      │
│  - Outbound Adapter (Durable Outbox)         │
└──────────────────────────────────────────────┘
      ▲                               │
      │ POST /api/v1/jobs/claim       │ POST /api/v1/jobs/:id/ack
      │ (rt_token)                    │ (status + results)
      │                               ▼
┌──────────────────────────────────────────────┐
│                MeWork Daemon                 │
│  - Local AI CLI (claude/codex/opencode)      │
│  - Isolated workspace (~/.mework/work/)      │
└──────────────────────────────────────────────┘
```

- **Inbound triggers**: External task management systems (e.g., Mello) send webhooks to the MeWork Server (`POST /webhooks/{provider}`). The server validates and parses the webhook payload to enqueue jobs into the PostgreSQL job queue.
- **Daemon polling & execution**: The daemon polls the MeWork Server's `/api/v1/jobs/claim` endpoint using its secure `rt_token` to claim pending jobs, executes them locally via the local AI engine, and returns the status/results using the acknowledgement (`/ack`) endpoint.
- **Server-side write-backs**: The MeWork Server manages posting comments or updating status back to the external task system via a durable outbox queue using its configured provider adapters, removing any provider-specific logic (and MCP configurations) from the local daemon.

## Setup and Configuration

Before starting the daemon, you must establish connection configurations, register your runtime, and set up your system prompt instructions.

### 1. Central Server Endpoint
To specify the location of the central `mework-server`:
```bash
mework config set server_url http://localhost:8080
```
This is saved in `~/.mework/config.json` under `server_url`.

### 2. Provider Connection Setup
To allow the server to write comments and status updates back to your task boards, connect a provider account (e.g., Mello) to the central server:
```bash
# Set your local Mello session credentials first:
mework login --token mello_pat_xxx

# Connect the provider (omit --token to be prompted securely):
mework provider connect --provider mello --token mello_pat_xxx
```
*(The server stores this token securely encrypted with `MEWORK_SECRET_KEY` to perform outbound API calls on your behalf.)*

### 3. Runtime Registry & Token Config
Register your local machine runtime on the server to obtain a unique daemon execution token (`rt_token`):
```bash
mework runtime register --code local-macbook --label "My MacBook Pro"
```
Output:
```
Runtime registered successfully!
ID:    <uuid-id>
Code:  local-macbook
Token: mework_rt_xxx

IMPORTANT: Save the Token. It will NOT be shown again.
To configure this runtime for the daemon, run:
  mework config set rt_token mework_rt_xxx
```
Save the returned token:
```bash
mework config set rt_token mework_rt_xxx
```

To list registered runtimes or revoke an inactive runtime:
```bash
mework runtime list
mework runtime revoke --id <uuid-id>
```

### 4. AI Profile Configuration
Create server-side AI profiles that define system prompts, backend hints (e.g., `claude`, `codex`, `opencode`), and target harnesses (e.g., `claude-code`):
```bash
# Create a profile
mework profile create --name default --body path/to/system_prompt.txt --backend claude --harness claude-code

# List profiles
mework profile list

# Update an existing profile
mework profile update --name default --body path/to/new_prompt.txt --backend claude

# Delete a profile
mework profile delete --name default
```

## Daemon lifecycle

| Command | Behavior |
|---------|----------|
| `mework daemon start` | Re-execs detached in the background (`--foreground` runs in-process). No-op if already running. |
| `mework daemon stop` | Graceful shutdown via the local health port; falls back to SIGTERM. |
| `mework daemon status` | Reports running/stopped, pid, and health port. |
| `mework daemon restart` | Stops (if running) then starts. |
| `mework daemon logs [-f]` | Prints (and optionally follows) the daemon log. |

State and operational files live under the profile directory (default `~/.mework/`):

- `daemon.pid` — running process id (liveness checked via signal 0, so a stale file after a crash is not mistaken for a live daemon).
- `daemon.log` — daemon output.
- `work/<job-id>/` — isolated working directory per agent run.

Note: Trigger idempotency is managed entirely server-side (using PostgreSQL unique constraints on `(provider_code, external_event_id)` and advisory locks for active job claims), meaning the daemon no longer requires a local `state.json` file.

The health/shutdown port is derived deterministically from the profile name (base `19514` + hash), so each profile gets its own port without extra config.

## Trigger semantics

The daemon polls the server for jobs and executes them sequentially:

1. **Job Claim**: The daemon issues a poll request (`POST /api/v1/jobs/claim`) with its `rt_token`. When a job becomes available, the server locks and leases the job to the daemon.
2. **State Acknowledgment**: Before starting, the daemon acknowledges the job is running (`POST /api/v1/jobs/:id/ack` with status `running`).
3. **Heartbeat Loop**: While the job executes locally, the daemon runs a background ticker that heartbeats the server (`POST /api/v1/jobs/:id/heartbeat` every 30 seconds) to extend the lease and prevent a timeout/lease lapse.
4. **Execution**: The daemon builds the prompt from the canonical job payload (snapshot of title/description, instructions, profile prompt, and workflow config). It runs the AI CLI with the prompt supplied via stdin and captures the stdout/stderr and exit status inside an isolated workspace.
5. **Terminal Acknowledgment**: The daemon sends the execution result back to the server (`POST /api/v1/jobs/:id/ack` with a terminal status `done` or `failed` along with the execution output/summary).
6. **Server-Side Write-Back**: The server's Outbound Adapter picks up the finished job status and writes the comments/status update back to the external task system (e.g., Mello) via the provider REST API.

**Trigger Idempotency and Loop Prevention:**
Trigger idempotency is handled entirely on the server side using unique constraints on `(provider_code, external_event_id)` to ensure each webhook event triggers exactly one job. Furthermore, the server filters out events/comments authored by the agent itself to prevent infinite feedback loops.

## AI backends

The daemon executes the local AI CLI based on the target `harness` specified in the job payload (e.g., `claude-code`). Local AI CLI executables (like `claude`, `codex`, or `opencode`) are resolved from the local `PATH` or overridden via `daemon.backends` settings. The instructions profile, workflow config, and task snapshots are delivered directly within the job payload and fed strictly via stdin to ensure injection safety.

## Profiles

Profiles exist in two dimensions:

- **Local Daemon Profiles**: Specifying `--profile dev` on CLI commands isolates local daemon configuration, pid, logs, and workspace folders under `~/.mework/profiles/dev/`.
- **Server-Side Profiles**: Created and updated via the CLI (`mework profile create/update`) and stored on the central server. These store markdown instructions, backend hints, target harnesses, and workflow configuration JSON. When a job is enqueued, the server-side profile configuration is resolved, snapshotted, and delivered directly within the job payload to the daemon.

## Not yet implemented

- `mework update` self-update: deferred until the project has a published GitHub release. The `.goreleaser.yml` + `Makefile` provide the build machinery; the update download/verify/swap flow needs the real release repo first.
