# CLI and Agent Daemon Guide

Operational reference for the `mework` CLI and its agent-runtime daemon.

## Architecture

```
External Task System (e.g. Mello)
      │                               ▲
      │ webhook                       │ write-back (MCP / API)
      ▼                               │
┌──────────────────────────────────────────────┐
│                MeWork Server                 │
│  - Inbound Adapter                           │
│  - PostgreSQL Job Queue                      │
│  - Outbound Adapter (Durable Outbox)         │
└──────────────────────────────────────────────┘
      ▲                               │
      │ GET /v1/jobs/next             │ POST /v1/jobs/:id/ack
      │ (long-poll, rt_token)         │ (status + results)
      │                               ▼
┌──────────────────────────────────────────────┐
│                MeWork Daemon                 │
│  - Local AI CLI (claude/codex/opencode)      │
│  - Isolated workspace (~/.mework/work/)      │
└──────────────────────────────────────────────┘
```

- **Inbound triggers**: External task management systems (e.g., Mello) send webhooks to the MeWork Server. The server validates and parses the webhook payload to enqueue jobs into the PostgreSQL job queue.
- **Daemon polling & execution**: The daemon long-polls the MeWork Server's `/v1/jobs/next` endpoint using its secure `rt_token` to claim pending jobs, executes them locally via the local AI engine, and returns the result using the acknowledgement (`ack`) endpoint.
- **Server-side write-backs**: The MeWork Server manages posting comments or updating status back to the external task system via a durable outbox queue using its configured provider adapters, removing any provider-specific logic (and MCP configurations) from the local daemon.

## Daemon lifecycle

| Command | Behavior |
|---------|----------|
| `mework daemon start` | Re-execs detached in the background (`--foreground` runs in-process). No-op if already running. |
| `mework daemon stop` | Graceful shutdown via the local health port; falls back to SIGTERM. |
| `mework daemon status` | Reports running/stopped, pid, and health port. |
| `mework daemon restart` | Stops (if running) then starts. |
| `mework daemon logs [-f]` | Prints (and optionally follows) the daemon log. |

State and operational files live under the profile directory (default `~/.mework/`):

- `daemon.pid` — running process id (liveness checked via signal 0, so a stale
  file after a crash is not mistaken for a live daemon).
- `daemon.log` — daemon output.
- `work/<task-id>/` — isolated working directory per agent run.

Note: Trigger idempotency is managed entirely server-side (using PostgreSQL unique constraints on `(provider_code, external_event_id)` and advisory locks for active job claims), meaning the daemon no longer requires a local `state.json` file.

The health/shutdown port is derived deterministically from the profile name
(base `19514` + hash), so each profile gets its own port without extra config.

## Trigger semantics

The daemon long-polls the server for jobs, executing them sequentially:

1. **Job Claim**: The daemon issues a long-poll request (`GET /v1/jobs/next?wait=25s`) with its `rt_token`. When a job becomes available, the server locks and leases the job to the daemon.
2. **State Acknowledgment**: Before starting, the daemon acknowledges the job is running (`POST /v1/jobs/:id/ack` with status `running`).
3. **Heartbeat Loop**: While the job executes locally, the daemon runs a background ticker that heartbeats the server (`POST /v1/jobs/:id/heartbeat` every ~30 seconds) to extend the lease and prevent a timeout/lease lapse.
4. **Execution**: The daemon builds the prompt from the canonical job payload (snapshot of title/description, instructions, profile prompt, and workflow config). It runs the AI CLI with the prompt supplied via stdin and captures the stdout/stderr and exit status inside an isolated workspace.
5. **Terminal Acknowledgment**: The daemon sends the execution result back to the server (`POST /v1/jobs/:id/ack` with a terminal status `done` or `failed` along with the execution output/summary).
6. **Server-Side Write-Back**: The server's Outbound Adapter picks up the finished job and enqueues it into a durable outbox queue. The server then writes the comments/status update back to the external task system (e.g., Mello) via the provider's MCP/REST APIs.

**Trigger Idempotency and Loop Prevention:**
Trigger idempotency is handled entirely on the server side using unique constraints on `(provider_code, external_event_id)` to ensure each webhook event triggers exactly one job. Furthermore, the server filters out events/comments authored by the agent itself to prevent infinite feedback loops.

## AI backends

The daemon executes the local AI CLI based on the target `harness` specified in the job payload (e.g., `claude-code`). Local AI CLI executables (like `claude`, `codex`, or `opencode`) are resolved from the local `PATH` or overridden via `daemon.backends` settings. The instructions profile, workflow config, and task snapshots are delivered directly within the job payload and fed strictly via stdin to ensure injection safety.

## Profiles

Profiles exist in two dimensions:

- **Local Daemon Profiles**: Specifying `--profile dev` on CLI commands isolates local daemon configuration, pid, logs, and workspace folders under `~/.mework/profiles/dev/`.
- **Server-Side Profiles**: Created and updated via the CLI (`mework profile add`) and stored on the central server. These store markdown instructions, backend hints, target harnesses, and workflow configuration JSON (e.g., allowed tools, prompt templates, or environment options). When a job is enqueued, the server-side profile configuration is resolved, snapshotted, and delivered directly within the job payload to the daemon.

## Not yet implemented

- `mework update` self-update: deferred until the project has a published GitHub
  release. The `.goreleaser.yml` + `Makefile` provide the build machinery; the
  update download/verify/swap flow needs the real release repo first.
- Checklist write-back tick: the MCP client supports checklist tools, but the
  daemon does not auto-tick a checklist item because there is no generic
  "agent done" item convention on a Mello board. Wire it via `done_column_id`
  or a future board-convention setting.
