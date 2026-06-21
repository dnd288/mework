# mework

An **agent hub** for running AI coding agents from your task board — built around
the **DX of a GitHub Actions self-hosted runner**: install a runner once, then
drive everything remotely from the hub. Pull new agents on demand and run any
*permitted* operation **without ever operating on the client machine** again.

> **Status.** The **overview and architecture below describe the target design**
> (the proposed redesign). The **current code** still implements an earlier
> pull-based pipeline (webhook → Postgres job queue → 5s-poll daemon →
> host-subprocess execution → REST write-back) — see
> [Current implementation](#current-implementation-today). The redesign is tracked
> as five OpenSpec changes under `openspec/changes/` and detailed in
> [docs/architecture.md](docs/architecture.md).

## Overview

MeWork connects task-management platforms (Mello kanban today; Jira / Linear /
GitHub Issues by design) to AI coding agents that run on **your** machines — not in
a cloud AI service — so source code and credentials stay local. It is built from
**three components**:

1. **Server = Agent Hub** — a provider-agnostic brain: a **registry** of runners and
   sessions, a **publisher/broker** that pushes work to **topics**, an **agent
   catalog** of versioned, pullable agents, an **orchestrator**, and a
   **permission/policy** engine that scopes what each dispatched agent may do.
2. **Daemon = Runner** — installed **once** on a device, then unattended: it
   **subscribes over SSE**, **pulls** the agent it's been dispatched, runs it in a
   sandbox, enforces the granted permissions, and reports the result.
3. **Sandbox** — a **pluggable isolated runtime** (`local` / `docker` / …) that runs
   **one agent**, so dispatched work executes without touching the host directly.

The model is a **hybrid**: the hub routes and orchestrates (it never sees your
source); the agents execute locally inside sandboxes under least-privilege grants.

## Architecture

```
provider ─webhook→ Server (Agent Hub)
                     • catalog: versioned, pullable agents
                     • registry + permission/policy engine (permitted ops)
                     • publisher/broker: topics (pluggable backend)
                     • orchestrator + session manager
                     │  SSE stream  (server ─push→ runner)
                     ▼
                   Daemon = Runner  (enrolled once, unattended)
                     • subscribe topics over SSE
                     • on dispatch: PULL agent ─GET→ catalog
                     • enforce granted permissions
                     │ spawn (one agent per sandbox)
                     ▼
                   Sandbox (local | docker | …)
                     • isolated; runs ONE agent
                     • result ─POST→ hub ─REST writeback→ provider
```

**Client transport is SSE only.** Runners subscribe over a single Server-Sent
Events stream (`text/event-stream`); the hub pushes work as it's published, and a
reconnecting runner resumes with `Last-Event-ID`. The reverse direction — acks,
results, agent pulls — is ordinary POST/GET. The hub's internal broker (queue,
stream, or DB; default Postgres `LISTEN/NOTIFY`) is an implementation detail behind
this contract.

**Permissions: "any *permitted* operation."** Every dispatch carries a scoped,
least-privilege **grant**. The hub authorizes, the runner enforces locally, and the
sandbox contains — three layers of defense. No grant for an operation means it's
denied.

Full design, the SSE contract, the permission model, and the current→target
migration map: **[docs/architecture.md](docs/architecture.md)**.

## Spec-driven development with OpenSpec

This project uses **[OpenSpec](https://github.com/Fission-AI/OpenSpec)** — every
non-trivial change starts as a spec/proposal, not code.

```
/opsx:explore   think through an idea (no code written)
/opsx:propose   create a change + its artifacts (proposal → delta specs → design → tasks)
/opsx:spec      cross-validate the spec across 6 quality axes → revise until clean
/opsx:apply     implement the tasks, ticking them off
/opsx:sync      merge the change's delta specs into the main specs
/opsx:archive   finalize → openspec/changes/archive/
```

Canonical, already-implemented capabilities live as baseline specs in
`openspec/specs/` (`provider-gateway`, `webhook-pipeline`, `job-queue`,
`rest-writeback`, `daemon-runtime`, `cli`, `auth-and-secrets`). The redesign above
is captured as five active changes under `openspec/changes/`, landed in order:

| Change | Adds | Status |
|--------|------|--------|
| `c0002-repo-restructure` | **lands first** — `shared`/`client`/`server`/`platform` domains, enforced dependency rule, per-component build/test | proposed |
| `c0002-message-bus` | SSE pub/sub transport (topics, subscribe, resumable, pluggable broker) | proposed |
| `c0003-agent-catalog` | versioned pullable agents + permission/policy grants | proposed |
| `c0004-agent-runner` | install-once enrollment + SSE pull→run→report loop | proposed |
| `c0005-sandbox-runtime` | pluggable isolated execution (`local`/`docker`/…) | proposed |

Change directories are named `cNNNN-<slug>` to encode apply order (lower lands
first; the leading `c` is required because OpenSpec change names must start with a
letter). A clean module structure first lets large teams develop, test, release,
and extend each component in parallel; the feature changes rebase onto it.

Workflow and spec-format details: [docs/openspec-workflow.md](docs/openspec-workflow.md).

---

## Current implementation (today)

> The sections below document the **currently built** pull-based pipeline. The CLI
> command surface (enrollment, agent catalog, sandboxes) will change as the
> redesign lands.

### Install

```bash
make build        # produces ./bin/mework and ./bin/mework-server
# or
go install ./cmd/...
```

### Quick start

```bash
# 1. Point the CLI to the central MeWork server.
mework config set server_url http://localhost:8080

# 2. Authenticate with your Mello personal access token.
mework login --token mello_pat_xxx
# (omit the value to be prompted, keeping the token out of shell history)

# 3. Connect a third-party provider account (e.g., mello) to the server.
mework provider connect --token mello_pat_xxx

# 4. Register this local daemon runtime to get a runtime token (rt_token).
mework runtime register --code local-macbook
mework config set rt_token mework_rt_xxx

# 5. Create an AI instruction profile on the server.
mework profile create --name default --body path/to/system_prompt.txt --backend claude --harness claude-code

# 6. Start the agent daemon.
mework daemon start          # background; --foreground to run in-process
mework daemon status
mework daemon logs -f

# 7. Trigger an agent run: comment "@mework <profile> [workflow] <instructions>" on any ticket.
#    e.g. "@mework default review fix the failing login test"
#    The server receives the webhook, enqueues the job, the daemon claims & executes it, and the server writes back the result.
```

### Commands

| Group | Commands |
|-------|----------|
| Core | `workspace list`, `board list/get`, `ticket list/get/create/move`, `comment list/add`, `search` |
| Runtime | `daemon start/stop/status/restart/logs`, `runtime register/list/revoke`, `profile create/list/update/delete` |
| Additional | `login`, `auth status/logout`, `config show/set`, `provider connect`, `version` |

Most list/get commands accept `--json`. Global flags: `--server-url`,
`--workspace-id`, `--profile`, `--debug`.

### How the trigger works

The central `mework-server` receives webhook events from the connected provider and
parses comments against the trigger grammar `@mework [profile] [workflow]
[instructions]`, where `workflow` (when present) is one of `plan`, `cook`, `test`,
`review`, `ship`, `journal`. A comment fires a job when:

- its body matches the `@mework` trigger grammar, **and**
- it was **not** authored by the daemon's own user (prevents self-retrigger loops), **and**
- it has not already been handled (unique constraint on `(provider_code, external_event_id)`).

The prompt is fed to the AI CLI over **stdin** (never as a shell argument) inside an
isolated workspace.

> The `daemon.trigger_keyword` config is a **legacy** local-daemon setting; the live
> webhook pipeline matches the `@mework` grammar above.

### Configuration

Config lives at `~/.mework/config.json` (use `--profile <name>` to isolate config,
daemon state, pid, and logs under `~/.mework/profiles/<name>/`). Resolution
precedence is **flag > environment > config file**.

| Key / Env | Purpose |
|-----------|---------|
| `MELLO_API_KEY` / `token` | Bearer token for REST |
| `MELLO_BASE_URL` / `base_url` | REST base (default `https://mello.mezon.vn/api/v1`) |
| `server_url` / `MEWORK_SERVER_URL` | Mework central server endpoint (default `http://localhost:8080`) |
| `rt_token` | Runtime registry token for daemon polling and execution |
| `MELLO_WORKSPACE_ID` / `workspace_id` | Default workspace |
| `daemon.trigger_keyword` | Legacy local-daemon trigger keyword; the webhook pipeline uses `@mework` |
| `daemon.done_column_id` | Optional column to move finished tickets to |

See [docs/cli-and-usage.md](docs/cli-and-usage.md) for details.

### Development

```bash
make test     # go test -p 1 ./...  (serialized: DB-backed tests share one Postgres)
make test-db  # start a Docker Postgres for DB-backed tests
make vet      # go vet ./...
make build    # build with version ldflags
make snapshot # goreleaser cross-compile (requires goreleaser)
```

DB-backed tests skip unless `TEST_DATABASE_URL` is set (e.g.
`postgres://postgres:postgres@localhost:5432/mework_test`).

## Docs

Start at **[docs/README.md](docs/README.md)** — the documentation index with reading
paths for users, operators, and contributors. Highlights:

- [docs/product-overview.md](docs/product-overview.md) — product overview (current + planned).
- [docs/architecture.md](docs/architecture.md) — agent-hub architecture + Today → Target migration.
- [docs/api-reference.md](docs/api-reference.md) — endpoints, auth, topics, data model.
- [docs/cli-and-usage.md](docs/cli-and-usage.md) — CLI reference + end-to-end usage.
- [docs/auth-and-secrets.md](docs/auth-and-secrets.md) — auth, tokens, grants, env vars.
- [docs/runtime-and-sandbox.md](docs/runtime-and-sandbox.md) — runner loop + sandboxes.
- [docs/deployment-guide.md](docs/deployment-guide.md) — deploy `mework-server`.
- [docs/openspec-workflow.md](docs/openspec-workflow.md) — spec-driven development workflow.
- [tests/e2e/README.md](tests/e2e/README.md) — interface-first BDD E2E scenario suite.
- [CLAUDE.md](CLAUDE.md) — architecture + developer guide for AI agents.
