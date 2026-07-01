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

Go module `mework` (Go 1.26), a `go.work` workspace. Binaries under `apps/`, shared code
under `libs/{client,server,shared,sandbox,tests,tools}`:

- **`apps/mework`** — the CLI, the local agent daemon, **and** `mework server start`
  (run the hub in-process).
- **`apps/mework-server`** — the standalone provider-gateway HTTP server.
- **`libs/sandbox/cmd/mework-sandbox`** — standalone sandbox runner *(stub today)*.

Two delivery flows coexist: the **legacy poll/queue pipeline** below (default webhook path),
and the **interactive session/sandbox** flow (`runner enroll` → daemon SSE → `sandbox start` /
`session create` → long-lived sandbox driven over the bus). End-to-end (legacy) flow:

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
- **`rt_token`** (runtime token, via `runner enroll`) guards daemon routes
  (`/api/v1/jobs/*`, `/api/v1/runners/sessions/*`, agent pull).
- `/webhooks/{provider}` is signature-verified, not token-auth'd. `/healthz`, `/livez`,
  `/readyz` are open.

## Repository layout

| Path | Responsibility |
|------|----------------|
| `apps/mework/` | CLI + daemon entrypoint + `server start` (in-process hub via `cli.SetServerStarter`) |
| `apps/mework-server/` | Standalone server entrypoint: load config → migrate → chi server with graceful shutdown |
| `libs/client/cli/` | cobra commands `cmd_*.go` (board, ticket, workspace, daemon, runner, runtime, agent, session, sandbox, server, profile, provider, auth, config, version) + config persistence (`~/.mework/`) |
| `libs/client/runner/` | daemon lifecycle (pid/health), SSE `Engine` (dispatch loop), one-shot + interactive `Session` execution |
| `libs/client/{enroll,subscribe,catalog,workspacefs}/` | runner enrollment, SSE/HTTP client, definition resolvers (HTTP/file), workspace artifact I/O |
| `libs/shared/{core,transport,config,grant}/` | core types, wire contract, config, grants |
| `libs/shared/providers/mello/` | Mello REST API client + models |
| `libs/server/hub/` | chi router, config, `/healthz` `/livez` `/readyz`, server assembly |
| `libs/server/auth/` · `libs/server/middleware/` | PAT authenticator; runtime (`rt_token`) + grant middleware |
| `libs/server/{registry,connection,catalog}/` | runtimes/enrollment, provider connections, agent catalog + AI profiles |
| `libs/server/{session,bus,orchestrator,channel}/` | session manager, message broker (memory/postgres), job lifecycle, channel routing |
| `libs/server/webhook/` | `/webhooks/{provider}` handler, signature verify, `ParseTrigger`, enqueue |
| `libs/server/writeback/` · `libs/server/provider/` | REST write-back outbox; provider adapter registry (`provider/mello`; github/jira stubs) |
| `libs/server/platform/{store,secret,token}/` | Postgres pool + goose migrations; AES-256-GCM seal/unseal; HMAC token hashing |
| `libs/sandbox/` | engines (`local`/`docker`/cloudflare/custom), runtime manager, agent detection |
| `libs/tests/{integration,e2e}/` | DB-backed integration tests; `e2e` BDD suite (behind the `e2e` build tag) |

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
  See `libs/sandbox/engine/local/runner.go`.
- **Job state machine is transactional with row locks; terminal states are
  immutable.** Allowed: `queued→claimed|failed`, `claimed→running|done|failed|queued`,
  `running→done|failed|queued`. Same-status transition is a no-op. See
  `libs/server/orchestrator/state.go`.
- **Webhook de-dup** relies on `UNIQUE(provider_code, external_event_id)`.
- **One active job per runtime** (partial unique index); claims use
  `FOR UPDATE SKIP LOCKED`.
- **Self-retrigger guard**: never enqueue a job for a comment authored by the
  daemon's own provider user.
- **Provider-agnostic schema**: identify external entities by
  `(provider_code, external_*_id)` — adding a provider must not require a
  migration. Add a new adapter under `libs/server/provider/<name>/`.
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

### Agent-hub redesign (shipped)

The **agent hub** redesign has largely **shipped**: the repo restructure
(`apps/`+`libs/`), install-once **`runner enroll`**, the daemon **subscribing over SSE**
(`runner.<id>.dispatch`), the versioned **agent catalog** + grant model, **interactive
sessions** and **workspace-bound sandboxes** in **pluggable engines** (`local`/`docker`).
The **legacy poll/queue pipeline** also still ships and is the **default** webhook path
(channel routing is off by default). Both coexist — see
[docs/architecture.md](docs/architecture.md).

Still stub / not production (treat as unbuilt): the artifact store (dummy), the NATS bus
backend, GitHub/Jira provider adapters, the standalone `mework-sandbox` binary, and the
cloudflare/custom engines. Shipped OpenSpec changes are archived under
`openspec/changes/archive/`.

## Gotchas

- **Trigger grammar is `@mework [profile] [workflow] [instructions]`** (see
  `libs/server/webhook/parse.go`), where workflow ∈
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
