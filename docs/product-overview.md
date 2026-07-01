# MeWork — Product Overview

> Canonical product overview. Where any older document (including the legacy
> Vietnamese PRD that this doc set replaces) disagrees with this file or the code,
> this document and the code are authoritative.

## What it is

**MeWork** is an AI-agent runtime and provider-gateway system. It automates
software-development workflows by connecting task-management platforms — Mello
kanban today, and by design Jira, Linear, and GitHub Issues — directly to AI coding
CLIs (Claude Code, Codex, OpenCode) running locally on a developer's machine.

## The problem

Centralized, cloud-hosted AI coding services require shipping your source code and
secret API keys off-machine, which many teams can't or won't do. Teams still want to
trigger AI work directly from where they already plan it: their kanban / issue board.

## The approach — a hybrid model

- A **central server** (`mework-server`) is a provider-agnostic router. It knows
  nothing about your source code.
- The **actual AI work runs locally** on the developer's machine, against local
  source code.
- Provider credentials are sealed server-side and used only to write results back;
  the local side never holds them.

The result: trigger an AI agent by commenting on a ticket, while source code and
credentials stay on machines you control.

## Who it's for

Developers and teams who want to drive AI coding agents from their kanban/issue
boards without handing their source code or API keys to a cloud AI provider.

## How a run works

1. A user comments `@mework <profile> [workflow] <instructions>` on a ticket.
2. The provider (Mello) sends a webhook to `mework-server`.
3. The server verifies the signature, parses the trigger, and routes the work —
   deduplicated so redelivered webhooks don't double-run.
4. A local runner receives the work, runs the selected AI CLI against the ticket in
   an isolated workspace, feeding the prompt over stdin.
5. The runner reports the result; the **server** writes it back to the ticket over
   the provider's REST API via a durable outbox.

> **Today vs target.** Step 3 today *enqueues a Postgres job* that the local daemon
> *long-polls and claims*; in the target architecture it *publishes to a topic* the
> runner *subscribes to over SSE*. Either way, the user experience is identical.
> See [architecture.md](architecture.md) for the migration.

## Trigger grammar

```
@mework <profile> [workflow] <free instructions>
```

- `profile` — the AI instruction profile to use (first token).
- `workflow` — optional; one of `plan`, `cook`, `test`, `review`, `ship`, `journal`
  when present as the second token (case-insensitive).
- everything after that is free-form instructions.

Examples:
- `@mework dev review fix the login bug` → profile `dev`, workflow `review`.
- `@mework dev fix the type errors` → profile `dev`, no workflow.

See [cli-and-usage.md](cli-and-usage.md) for the full walkthrough.

## Core capabilities

These map to the baseline specs in `openspec/specs/` (current, `[Implemented]`):

| Capability | What it does |
|------------|--------------|
| `provider-gateway` | Provider-agnostic adapter registry + per-account provider connections |
| `webhook-pipeline` | Inbound webhook ingestion, signature verify, trigger parsing, idempotent intake |
| `job-queue` | Durable Postgres job lifecycle: enqueue, claim, ack, heartbeat, state machine, sweeper |
| `rest-writeback` | Server-side, durable, exactly-once result write-back over the provider's REST API |
| `daemon-runtime` | Local poll worker that runs AI CLIs safely in isolated workspaces |
| `cli` | `mework` command surface + config resolution |
| `auth-and-secrets` | PAT vs runtime-token auth, AES-256-GCM credential sealing, HMAC token lookup |

## Non-functional priorities

- **Security** — source code and credentials stay local; provider credentials sealed
  with AES-256-GCM and used only at write-back; prompts fed over stdin (not argv) to
  avoid injection; restrictive file permissions (`0600`/`0700`). See
  [philosophy.md](philosophy.md).
- **Reliability** — durable Postgres job queue with a transactional state machine,
  heartbeats/leases, a crash sweeper, idempotent enqueue, and a durable write-back
  outbox for exactly-once delivery.
- **Provider-agnostic** — new providers plug in as adapters with no schema or runner
  changes.

## Status: today vs target

**`[Implemented]` today** — the full webhook → enqueue → claim → ack → REST
write-back pipeline, runtime/profile/connection management, the CLI, and a green
end-to-end test (`internal/integration/`).

**`[Planned]` — the agent-hub redesign.** A redesign is underway to move from the
current pull/poll model to an **agent hub** with the DX of a GitHub Actions
self-hosted runner: install a runner once, then drive everything remotely —

1. **Server = agent hub** — registry, orchestrator, an SSE **publisher/broker** with
   **topics**, a session manager, a versioned **agent catalog**, and a
   **permission/policy** engine.
2. **Daemon = runner** — enrolled once, then unattended: **subscribes over SSE**,
   **pulls** dispatched agents, enforces scoped grants, manages local sandboxes.
3. **Sandbox** — a pluggable isolated runtime (`local`/`docker`/…), one agent per
   sandbox.

Tracked as five OpenSpec changes under `openspec/changes/` (`c0002-repo-restructure`
… `c0005-sandbox-runtime`). Full design in [architecture.md](architecture.md).

**Superseded** — the original MCP-based write-back (now server-side REST) and the
original poll-Mello-directly daemon (now webhook-driven server + local runner).

## Related docs

- [architecture.md](architecture.md) — agent-hub architecture + Today → Target migration.
- [cli-and-usage.md](cli-and-usage.md) — install, configure, and trigger agents.
- [api-reference.md](api-reference.md) — endpoints, auth, topics, data model.
- [auth-and-secrets.md](auth-and-secrets.md) — authentication and secrets.
- [deployment-guide.md](deployment-guide.md) — server deployment.
- [openspec-workflow.md](openspec-workflow.md) — spec-driven development workflow.
