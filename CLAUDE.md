# CLAUDE.md

Guidance for AI coding agents (and humans) working in this repository.

## What this project is

`mework` is an **AI-agent runtime daemon + provider-gateway server**. It connects
task-management platforms (Mello kanban today; Jira / Linear / GitHub Issues by
design) to AI coding CLIs that run **locally** on a developer's machine.

The model is deliberately **hybrid**: a central server handles routing and a
durable job queue (provider-agnostic), while the actual AI work is pulled down
and executed on the developer's own machine via a local daemon. Source code and
provider credentials stay local instead of going to a cloud AI service.

See [docs/product-overview.md](docs/product-overview.md) for the full product
description, actors, and use cases.

## Architecture at a glance

Two binaries (Go module `mework`, Go 1.25.7):

- **`cmd/mework`** — the CLI **and** the local agent daemon.
- **`cmd/mework-server`** — the central provider-gateway HTTP server.

End-to-end flow:

```
Mello (kanban)
  │  user comments "@mework <profile> [workflow] <instructions>" on a ticket
  ▼
POST /webhooks/{provider}        (mework-server)
  │  adapter verifies signature, ParseTrigger matches the grammar
  ▼
jobs.Enqueue  ──▶  Postgres `jobs` (status=queued, deduped on provider_code+external_event_id)
  ▲                                   │
  │ rt_token auth                     │ long-poll claim (FOR UPDATE SKIP LOCKED)
  ▼                                   ▼
mework daemon  ──▶ claim → ack running → heartbeat (30s) → run AI CLI → ack done/failed
  │                                   (prompt via STDIN, isolated workdir, 30m timeout)
  ▼
server: durable outbox  ──▶  provider REST API (e.g. Mello CreateComment)  ──▶ result posted back
```

Two token types:
- **PAT** (Mello personal access token) guards management routes (`/api/v1`
  runtimes, connections, profiles).
- **`rt_token`** (runtime token) guards daemon job routes (`/api/v1/jobs/*`).
- `/webhooks/{provider}` is signature-verified, not token-auth'd. `/healthz` is open.

## Repository layout

| Path | Responsibility |
|------|----------------|
| `cmd/mework/` | CLI + daemon entrypoint; cobra commands `cmd_*.go` (board, ticket, auth, provider, runtime, profile, daemon, version) |
| `cmd/mework-server/` | Server entrypoint: load config → run migrations → start chi HTTP server with graceful shutdown |
| `internal/cli/` | Config struct & persistence (`~/.mework/config.json`), flag/env/file resolution, profile paths |
| `internal/mello/` | Mello REST API client + models (workspaces, boards, tickets, comments, search) |
| `internal/meworkclient/` | HTTP client for `mework-server` (jobs claim/ack/heartbeat, connections, profiles, runtimes) |
| `internal/daemon/` | Daemon lifecycle (pid/health), poll loop, prompt building & result formatting |
| `internal/agentrun/` | Detects installed AI CLIs and executes them (prompt via stdin, isolated workdir) |
| `internal/store/` | Postgres pgx pool + embedded goose migrations (`migrations/*.sql`) |
| `internal/server/` | chi router, config, `/healthz` |
| `internal/server/auth/` | PAT authenticator middleware |
| `internal/server/middleware/` | Runtime (`rt_token`) authenticator middleware |
| `internal/server/registry/` | Runtimes CRUD |
| `internal/server/connection/` | Provider connection CRUD (sealed credentials) |
| `internal/server/profile/` | AI profile CRUD |
| `internal/server/webhook/` | `/webhooks/{provider}` handler, signature verify, `ParseTrigger`, enqueue |
| `internal/server/jobs/` | Job lifecycle: enqueue, claim, ack, heartbeat, state machine, sweeper, write-back |
| `internal/server/provider/` | Provider adapter interface + registry; `provider/mello/` is the first adapter |
| `internal/server/secret/` | AES-256-GCM seal/unseal for credentials |
| `internal/server/token/` | Runtime token generation + HMAC-SHA256 lookup hashing |
| `internal/integration/` | End-to-end pipeline test (`TestFullPipelineE2E`) |

## Build, test, run

```bash
make build        # → bin/mework and bin/mework-server (version ldflags)
make vet          # go vet ./...
make test         # go test -p 1 ./...   (serialized: tests share one Postgres DB)
make test-db      # docker Postgres for DB-backed tests
make snapshot     # goreleaser cross-compile (CLI only)
```

- **Tests are run with `-p 1`** (serialized) because DB-backed tests share a
  single Postgres database. Don't parallelize them.
- **DB-backed tests skip** unless `TEST_DATABASE_URL` is set. Start Postgres with
  `make test-db`, then e.g.
  `export TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/mework_test`.
  Tests run migrations themselves.
- Tests are table-driven where practical and use `net/http/httptest` for HTTP.

### Running the server locally

Required env (server fails fast without these):

| Env | Purpose |
|-----|---------|
| `DATABASE_URL` | Postgres DSN (required) |
| `SERVER_KEY` | HMAC key for `rt_token` lookup hashing (required) |
| `MEWORK_SECRET_KEY` | AES-256-GCM key for sealing provider credentials (required) |
| `LISTEN_ADDR` | optional, default `:8080` |
| `WEBHOOK_SECRET` | optional |
| `MELLO_BASE_URL` | optional, default `https://mello.mezon.vn/api/v1` |

Migrations run automatically on startup. See
[docs/deployment-guide.md](docs/deployment-guide.md).

## Conventions & invariants (don't break these)

- **Prompts go to AI CLIs over stdin, never argv.** Ticket content is
  attacker-controllable; keeping it out of the command line avoids injection.
  See `internal/agentrun/runner.go`.
- **Job state machine is transactional with row locks; terminal states are
  immutable.** Allowed: `queued→claimed|failed`, `claimed→running|done|failed|queued`,
  `running→done|failed|queued`. Same-status transition is a no-op. See
  `internal/server/jobs/state.go`.
- **Webhook de-dup** relies on `UNIQUE(provider_code, external_event_id)`.
- **One active job per runtime** (partial unique index); claims use
  `FOR UPDATE SKIP LOCKED`.
- **Self-retrigger guard**: never enqueue a job for a comment authored by the
  daemon's own provider user.
- **Provider-agnostic schema**: identify external entities by
  `(provider_code, external_*_id)` — adding a provider must not require a
  migration. Add a new adapter under `internal/server/provider/<name>/`.
- **Credentials**: sealed with AES-256-GCM at rest, unsealed only server-side at
  write-back time. The daemon never holds provider credentials.
- **File perms**: `0600` for credential/config files, `0700` for dirs.

## Spec-driven development with OpenSpec

This project uses **[OpenSpec](https://github.com/Fission-AI/OpenSpec)** for
spec-driven development. **Start non-trivial work with a change proposal, not
code.**

- `/opsx:explore` — think through an idea (no code is written in this mode).
- `/opsx:propose "<name>"` — create a change and generate its artifacts
  (proposal → delta specs → design → tasks). Single-pass draft.
- `/opsx:spec "<name>"` — quality-gate the draft: cross-validate across the 6
  spec-review axes (in parallel) and revise until clean, before implementing.
- `/opsx:apply` — implement the change's tasks, ticking them off as you go.
- `/opsx:sync` — merge the change's delta specs into the main specs.
- `/opsx:ship-plan` — write a reviewable handoff under `.handoff/<change>/`: each
  task in `tasks.md` becomes a **test** task + a **code** task (the test plan + impl
  plan). No branch, no code.
- `/opsx:ship-code` — execute the handoff **test-first, per change task**: Red
  (failing test) → Green (impl, tick `tasks.md`) → **one commit** containing both →
  then Verify (`make vet`/`make test` + coverage + `openspec validate`) → Evidence →
  Sync → Changelog → push → open PR. Stops at PR opened (no auto-merge); `--dry-run`
  commits locally without push/PR, `--only <pair>`/`--retry-blocked` for resume.
- `/opsx:ship` — orchestrates `ship-plan` → review gate → `ship-code` in one go.
  Adds an AskUserQuestion at the top: **Local merge (no gh)** vs **Remote PR
  (gh pr create)**. Local path: branches → test-first per task → Local review →
  squash-merge into `main` → post-merge verify → sync delta specs → archive →
  optional tag → cleanup. Defaults to fully local (`noPushMain=true`); opt in
  to `git push origin main` with `--push-main`. The remote path ends at PR
  opened (no auto-merge); review threads are handled directly via `gh` from the
  PR UI rather than via a dedicated workflow.
- `/opsx:archive` — finalize and move the change to `openspec/changes/archive/`.
- `/opsx:ship-all` — auto-discover every ACTIVE OpenSpec change and ship the
  full project — branch → ship → archive — locally and **fully automatically (no
  confirmation, no per-change prompts)**, with halt-on-failure and idempotent
  resume. Each change keeps the full workflow (branch → a **few test-first units**
  → verify → review) and then **merges into `main` locally** (so the next
  dependency-ordered change builds on it) **and opens a PR** for the record/human
  review — result: the project on `main` plus one PR per change. Per change,
  decides mode from `openspec status` (apply+ship / spec+ship / ship-only /
  repair+ship / archive-only / skip). The orchestrator owns branch creation
  (`feat/<change>` from a clean `main`) and invokes the nested `ship-plan` /
  `ship-code` workflows via the `workflow()` helper; `ship-plan` groups the change
  into **a few units** (not one per tasks.md line) and `ship-code` implements each
  unit **test-first** (Red→Green→one commit per unit) — there is no standalone
  `/opsx:apply`. Sorted by cNNNN dependency order. Writes
  `openspec/changes/.ship-all-progress.json` as durable state. Honors
  `--from <cNNNN>`, `--only <list>`, `--dry-run` (opt-in plan-only), `--skip-spec`,
  `--bump {patch|minor|major}`, `--push-main`, `--no-archive`, `--merge-strategy`,
  `--force`. The skill `.claude/skills/openspec-ship-all/SKILL.md` is the source of
  truth for the per-change decision matrix.

Canonical (already-implemented) capabilities live as baseline specs in
`openspec/specs/<capability>/spec.md`:
`provider-gateway`, `webhook-pipeline`, `job-queue`, `rest-writeback`,
`daemon-runtime`, `cli`, `auth-and-secrets`. Read the relevant spec before
changing a subsystem, and update it (via a change's delta + sync) when behavior
changes.

Full workflow and format details: [docs/openspec-workflow.md](docs/openspec-workflow.md).

### Engineering practice skills (SDD + TDD)

The OpenSpec lifecycle is the spine; *how* to build well lives in engineering-practice
skills under `.claude/skills/` (adapted from
[addyosmani/agent-skills](https://github.com/addyosmani/agent-skills), MIT). The flow is
**spec → design → test → implement → verify → ship**, and it is **TDD-first**: write a
failing Go test, make it pass minimally, refactor while `make vet`/`make test` stay
green. Every shipped change leaves evidence in **`openspec/changes/<name>/evidence/`**
(`gates.md`, `test-results.md`, `coverage.txt`), linked from the PR. `using-agent-skills`
is the meta-router. See [docs/engineering-skills.md](docs/engineering-skills.md) for the
full map and the skills (12 vendored + the repo-authored `spec-review-and-quality`).

### Planned redesign (in proposal — not built yet)

A major redesign is proposed (not implemented): move from the current pull/poll
model to an **agent hub** with a GitHub-Actions-runner DX — install a runner once,
then **subscribe over SSE**, **pull** versioned agents from a catalog, run them in
**pluggable sandboxes** (`local`/`docker`/…), all under scoped permission grants.
It is captured as five OpenSpec changes under `openspec/changes/`
(`c0001-repo-restructure` … `c0005-sandbox-runtime`) and described in
[docs/architecture.md](docs/architecture.md). **The code still
implements the current poll/queue model** — do not assume the redesign exists when
working in the repo.

## Gotchas

- **Trigger grammar is `@mework [profile] [workflow] [instructions]`** (see
  `internal/server/webhook/parse.go`), where workflow ∈
  `plan|cook|test|review|ship|journal`. The older `/run` keyword is the legacy
  local-daemon `trigger_keyword` (config default) and is **not** what the current
  webhook pipeline matches.
- **Write-back is REST, not MCP.** `github.com/mark3labs/mcp-go` is still a
  dependency, but the daemon no longer does MCP write-back — the server posts the
  result over the provider's REST API.
- The canonical docs live under `docs/` ([docs/README.md](docs/README.md) is the
  index); they present the agent-hub target architecture with `[Implemented]` /
  `[Planned]` status badges. Where docs and code disagree, trust the code,
  `docs/architecture.md`'s "Implementation today" section, and the `openspec/specs/`
  baseline.
