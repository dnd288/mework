---
name: ci-cd-and-automation
description: Automates CI/CD pipeline setup, adapted for the Mezon Mentor Bot ("MeKnow") platform. Use when setting up or modifying build and deployment pipelines, the GitHub Actions workflow (.github/workflows/rag-engine.yml), quality gates, or the /opsx:ship verify step. Use when configuring the polyglot test runner (Python/uv, Go, TS/pnpm) with a pgvector service container or debugging CI failures.
tags: [infra]
---

# CI/CD and Automation

## Overview

Automate quality gates so that no change reaches `main` without passing the resolver-selected gates for every toolchain it touched. CI/CD is the enforcement mechanism for every other skill — it catches what humans and agents miss, and it does so consistently on every single change.

This is a **polyglot** repo (`mezon-bot-ai`, consumed as the `platform/` git submodule). A single change can touch Python (`uv` workspace), Go (standalone modules, go 1.24), and TypeScript (`apps/portal`, pnpm). The gate resolver (`.claude/workflows/lib/gate-resolver.js`) maps each touched path to its owning package (nearest enclosing manifest) and runs that toolchain's gates — there is no single `make vet`/`make test`.

**Shift Left:** Catch problems as early in the pipeline as possible. A bug caught in `ruff check` or `pyright` costs seconds; the same bug caught after merge costs hours. Move checks upstream — lint/typecheck before tests, tests before merge, merge before deploy. `/opsx:ship` runs these same resolver gates *before* it opens the PR, so failures are caught on your machine, not in review.

**Faster is Safer:** Smaller batches and more frequent releases reduce risk, not increase it. One OpenSpec change per PR is easier to debug than a quarter's worth of work. Frequent, small ships build confidence in the release process itself.

## When to Use

- Setting up or modifying the CI pipeline (`.github/workflows/rag-engine.yml`)
- Adding or modifying automated checks
- Configuring the pgvector service container for DB-backed tests
- When a change should trigger automated verification
- Debugging CI failures

## The Quality Gate Pipeline

Every change goes through its toolchain's gates before merge. The resolver runs only the gates for the toolchains a change actually touches:

```
Pull Request Opened
    │
    ▼
┌──────────────────────────────────────────────────────────┐
│  Python (uv dir D)                                         │
│    uv --directory D run ruff check .                       │
│    uv --directory D run ruff format --check .              │
│    uv --directory D run pyright                            │
│    uv --directory D run python -m pytest -q                │
│      ▲ DB-dependent tests need pgvector + TEST_DATABASE_URL│
│  Go (module dir M, go 1.24)                                │
│    go build ./...  →  go vet ./...  →  go test -race ./...  │
│  TS (apps/portal)                                          │
│    pnpm typecheck → pnpm lint → pnpm test                  │
│    pnpm test:e2e   (gated tier — not every micro-change)   │
│  Bench:  bash benchmarks/ci-free-gates.sh                  │
│  Always: openspec validate "<change>" --strict             │
└──────────────────────────────────────────────────────────┘
    │
    ▼
  Ready for review
```

**No gate can be skipped.** If `ruff`/`pyright`/`go vet` fails, fix the code — don't suppress the diagnostic. If a test fails, fix the code — don't add a skip marker. DB-dependent tests **skip themselves** when `TEST_DATABASE_URL` / pgvector is unavailable, so CI *must* provide them or the most important coverage silently disappears.

## GitHub Actions Configuration

### CI Pipeline — `.github/workflows/rag-engine.yml`

The platform's CI workflow has **four jobs**. The workflow file itself is maintained separately; this is its shape and intent.

```yaml
# .github/workflows/rag-engine.yml
name: rag-engine

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  # 1) Python test matrix with a pgvector service + migrations
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_DB: meknow_test
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    env:
      TEST_DATABASE_URL: postgres://postgres:postgres@localhost:5432/meknow_test
    steps:
      - uses: actions/checkout@v4
        with:
          submodules: recursive   # repo is the platform/ submodule of funix-mezon-bot
      - uses: astral-sh/setup-uv@v5
      - run: uv --directory apps/backend run alembic upgrade head
      - run: uv --directory <member> run python -m pytest -q   # per touched member

  # 2) Go worker module gates (go 1.24)
  go-worker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { submodules: recursive }
      - uses: actions/setup-go@v5
        with: { go-version: '1.24', cache: true }
      - run: go vet ./...
      - run: go test -race ./...

  # 3) Free benchmark ladder on every PR (deterministic, ~2min)
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { submodules: recursive }
      - run: bash benchmarks/ci-free-gates.sh

  # 4) LLM-tier judge gates — nightly / on main / workflow_dispatch, NOT per-PR
  judge-gates:
    if: github.ref == 'refs/heads/main' || github.event_name == 'workflow_dispatch'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { submodules: recursive }
      - run: bash benchmarks/gates/faithfulness-gte.sh 0.85
      - run: bash benchmarks/gates/citation-accuracy-gte.sh 0.95
      - run: bash benchmarks/gates/latency-p95-lte.sh 12000
```

Notes specific to this repo:
- **The repo is a git submodule** (`mezon-bot-ai`, mounted at `platform/` in the
  `funix-mezon-bot` superproject). CI must check out with `submodules: recursive`.
- **Migrations are an explicit step** for the Python `test` job:
  `uv --directory apps/backend run alembic upgrade head` against the pgvector
  service, before pytest. The Go worker job needs no DB.
- The plaintext `postgres/postgres` credential is fine for an ephemeral CI-only
  service container, but never reuse it for real secrets (those belong in GitHub
  Secrets).

### The LLM benchmark gates run on a different cadence

The deterministic `benchmark` job runs the **free ladder** (`benchmarks/ci-free-gates.sh`) on every Python/bench PR. The **LLM-tier** `judge-gates` (faithfulness >= 0.85, citation accuracy >= 0.95, p95 latency <= 12s) are expensive and non-deterministic — they run **nightly and on `main`/`workflow_dispatch`, not per-PR**. These thresholds are the product's real contract; their gate scripts live in `benchmarks/gates/` and back the spec scenarios. Cross-tenant isolation (`tenant-isolation-test.sh`) and ACL escape (`retrieve-kb-acl-test.sh`) are pass/fail and protect the `tenant_id`-everywhere and server-side-ACL invariants.

### The OpenSpec verify step

`/opsx:ship` runs its verify step by invoking the **resolver gates** for the touched toolchains — the same commands listed in the pipeline above (`ruff`/`ruff format --check`/`pyright`/`pytest` for Python members, `go build`/`go vet`/`go test -race` for Go modules, `pnpm typecheck`/`lint`/`test` for the portal, the bench ladder for bench changes) plus the always-on `openspec validate "<change>" --strict`. Green-locally on these gates is what green-in-CI looks like.

## Feeding CI Failures Back to Agents

The power of CI with AI agents is the feedback loop. When CI fails:

```
CI fails
    │
    ▼
Copy the failure output
    │
    ▼
Feed it to the agent:
"The rag-engine workflow failed with this error:
[paste specific error]
Reproduce locally with the owning toolchain's gate (e.g.
`uv --directory <member> run python -m pytest -q`, after
`alembic upgrade head` against pgvector), fix the root cause,
and verify locally before pushing again."
    │
    ▼
Agent fixes → pushes → CI runs again
```

**Key patterns:**

```
ruff / pyright failure  → Agent reads the diagnostic location and fixes it
go vet failure          → Agent reads the cited location and fixes it
Build / import error    → Agent checks imports, pyproject/go.mod, the failing package
pytest / go test fail   → Agent follows the debugging-and-error-recovery skill
pnpm test fail          → Agent checks the portal change + generated OpenAPI types
DB test "skipped"       → TEST_DATABASE_URL/pgvector wasn't available — service misconfigured
go test -race flake     → Concurrency bug in a worker; investigate, don't re-run blindly
judge-gate regression   → faithfulness/citation/latency dropped — a retrieval/prompt regression
```

To reproduce CI locally (Python example — start a pgvector container, migrate, then test):

```bash
docker run -d --name meknow-pg -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=meknow_test -p 5432:5432 pgvector/pgvector:pg16
export TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/meknow_test
uv --directory apps/backend run alembic upgrade head
uv --directory <member> run ruff check . && uv --directory <member> run pyright && \
  uv --directory <member> run python -m pytest -q
```

## Deployment & Release Strategy

The platform deploys the backend (`apps/backend`) and the Python/Go workers; the portal (`apps/portal`) is a separate front-end artifact. The release flow:

```
OpenSpec change approved
    │
    ▼
/opsx:ship  → verify (resolver gates) → sync → CHANGELOG → commit → push → PR
    │ STOPS here (no auto-merge)        (all inside the platform/ submodule)
    ▼
Human review + merge to main (submodule default branch)
    │
    ▼
/opsx:archive (post-merge)
    │
    ▼
Superproject gitlink bump (separate, MANUAL):
  cd <funix-mezon-bot> && git add platform && git commit   ← never auto-pushed
    │
    ▼
Deploy: backend + workers; alembic migrations run on deploy
```

### Faster is Safer, applied here

- One change per PR, inside the submodule. The `/opsx:ship` verify gate runs the
  same resolver gates CI runs, so green-locally usually means green-in-CI.
- Keep alembic migrations forward-only and additive where possible. Respect the
  **append-only versions** invariant: `BotVersion`/`KBVersion`/golden-set rows are
  new children with parent pointers, never in-place mutations.

### Rollback

Backend/worker rollback = redeploy the previous image. Because migrations are additive, prefer migrations safe to run against the previous build too. A bad gitlink bump in the superproject is rolled back by committing the previous `platform` SHA there — the submodule history is untouched.

## Environment Management

```
CI service pgvector   → ephemeral; TEST_DATABASE_URL points at it
DATABASE_URL          → deploy platform (production Postgres + pgvector DSN)
MINIMAX_API_KEY       → GitHub Secrets / deploy vault (Anthropic-compatible LLM endpoint)
LLM_BASE_URL          → deploy config (MiniMax via the Anthropic-compatible endpoint)
WEBHOOK_SECRET / etc. → GitHub Secrets / deploy vault
```

CI should never hold production secrets. The only secret the `test`/`go-worker`/`benchmark` jobs need is the throwaway Postgres password for the service container; the LLM key belongs only to the `judge-gates` tier.

## Automation Beyond CI

### Dependabot

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: gomod          # the Go worker modules (go 1.24)
    directory: /
    schedule: { interval: weekly }
    open-pull-requests-limit: 5
  - package-ecosystem: pip            # uv / Python workspace
    directory: /
    schedule: { interval: weekly }
  - package-ecosystem: npm            # apps/portal (pnpm)
    directory: /apps/portal
    schedule: { interval: weekly }
  - package-ecosystem: github-actions
    directory: /
    schedule: { interval: weekly }
```

### Build Cop Role

Designate someone responsible for keeping CI green. When the build breaks, the Build Cop's job is to fix or revert — not necessarily the person whose change caused the break. This prevents broken builds from accumulating while everyone assumes someone else will fix it.

### PR Checks (branch protection)

- **Required status checks:** `test`, `go-worker`, and `benchmark` must pass before merge (`judge-gates` is main/nightly, not a PR gate)
- **Required reviews:** at least 1 approval (`/opsx:ship` opens the PR but never merges)
- **Branch protection:** no force-pushes to `main` (the submodule's default branch)

## CI Optimization

When the pipeline gets slow, apply these in order of impact:

```
Slow CI pipeline?
├── Cache per-toolchain artifacts
│   └── setup-uv cache, setup-go cache: true, pnpm store cache
├── Run only the toolchains a change touched
│   └── The resolver already scopes gates by touched path; mirror it with CI path filters
├── Keep the LLM judge-gates off the PR path
│   └── They are nightly / on main — never block a PR on a non-deterministic LLM tier
├── Parallelize the jobs
│   └── test / go-worker / benchmark are independent — run them as parallel jobs
└── Use a larger runner
    └── For CPU-heavy compile or embedding work, a larger GitHub-hosted runner
```

> Do **not** move the expensive `judge-gates` LLM tier onto the per-PR path — it is non-deterministic and slow by design. Keep it nightly / on main.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "CI is too slow" | Cache per-toolchain artifacts and run only touched toolchains. Don't skip gates. |
| "This change is trivial, skip CI" | Trivial changes break builds. CI is fast for trivial changes anyway. |
| "The DB test is flaky, just re-run" | A `go test -race` flake usually hides a real worker race; a pytest flake often hides leaked tenant state. Investigate. |
| "We'll add CI later" | Projects without CI accumulate broken states. Keep all four jobs green from day one. |
| "I'll just lint, skip the DB tests" | DB-dependent tests cover retrieval/tenancy. Without pgvector + `TEST_DATABASE_URL` they silently skip — that's a gap, not a pass. |
| "Run the judge-gates on every PR" | They're non-deterministic and expensive. They run nightly / on main by design. |

## Red Flags

- `TEST_DATABASE_URL` / pgvector not available in the `test` job → DB-dependent tests silently skip
- `ruff` / `pyright` / `go vet` failures suppressed instead of fixed
- Tests skip-marked to make the pipeline green
- The LLM `judge-gates` tier moved onto the per-PR critical path
- Production secrets (`MINIMAX_API_KEY`, `DATABASE_URL`) in CI logs or workflow YAML
- `submodules: recursive` missing from `actions/checkout` → submodule content absent
- The superproject gitlink bump auto-pushed by an automated job (it is a manual step)
- No branch protection requiring `test` / `go-worker` / `benchmark`

## Project notes

- The CI workflow lives at `.github/workflows/rag-engine.yml` with four jobs:
  `test` (Python matrix + `pgvector/pgvector:pg16` service + `alembic upgrade
  head`), `go-worker` (go 1.24 `go vet` + `go test -race`), `benchmark` (free
  ladder per PR), and `judge-gates` (LLM tier on main/dispatch). Checkout uses
  `submodules: recursive` because the repo is the `platform/` submodule.
- **`/opsx:ship` runs the same verify gates** — the resolver-selected per-toolchain
  gates plus `openspec validate "<change>" --strict` — locally before committing,
  pushing, and opening the PR via `gh`, and it **STOPS at the opened PR** (no
  auto-merge). All of this happens inside the `platform/` submodule. `/opsx:archive`
  runs after the human merge; the superproject gitlink bump is a separate manual
  step.
- The LLM benchmark gates (faithfulness >= 0.85, citation >= 0.95, p95 <= 12s)
  run nightly / on main — they are the product contract but not a per-PR gate.
- Capture verification evidence under `openspec/changes/<name>/evidence/`.
- See the sibling `git-workflow-and-versioning` skill for branch/commit/PR
  discipline (and the submodule gitlink workflow) these gates enforce, and
  `documentation-and-adrs` for recording CI/infra decisions as ADRs or spec
  updates.

## Verification

After setting up or modifying CI:

- [ ] The four jobs (`test`, `go-worker`, `benchmark`, `judge-gates`) run with the right triggers
- [ ] `actions/checkout` uses `submodules: recursive`
- [ ] The `test` job has the `pgvector/pgvector:pg16` service, runs `alembic upgrade head`, and exports `TEST_DATABASE_URL`
- [ ] The LLM `judge-gates` tier is gated to main / `workflow_dispatch`, never per-PR
- [ ] Failures block merge (branch protection requires `test` / `go-worker` / `benchmark`)
- [ ] No production secrets in the workflow YAML or logs
- [ ] `openspec validate "<change>" --strict` runs regardless of toolchains touched
- [ ] CI results feed back into the development loop
