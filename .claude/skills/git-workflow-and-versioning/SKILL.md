---
name: git-workflow-and-versioning
description: Structures git workflow practices, adapted for the Mezon Mentor Bot ("MeKnow") platform — the platform/ git submodule of funix-mezon-bot. Use when making any code change. Use when committing, branching, resolving conflicts, writing Conventional Commit messages, opening PRs via gh, bumping the superproject gitlink, maintaining CHANGELOG.md, or organizing work across parallel streams.
tags: [infra]
---

# Git Workflow and Versioning

## Overview

Git is your safety net. Treat commits as save points, branches as sandboxes, and history as documentation. With AI agents generating code at high speed, disciplined version control is the mechanism that keeps changes manageable, reviewable, and reversible.

**This repo is a git submodule.** `mezon-bot-ai` is consumed as the `platform/` submodule of the `funix-mezon-bot` superproject (customer pack). All branches, PRs, and merges in this skill happen **inside the submodule** (default branch `main`). Updating the superproject to point at a new submodule commit — the **gitlink bump** — is a separate, manual step that is **never** automated (see "The Submodule Gitlink Bump").

## When to Use

Always. Every code change flows through git.

## Core Principles

### Trunk-Based Development (Recommended)

Keep `main` always deployable. Work in short-lived feature branches that merge back within 1-3 days. Long-lived development branches are hidden costs — they diverge, create merge conflicts, and delay integration. DORA research consistently shows trunk-based development correlates with high-performing engineering teams.

```
main ──●──●──●──●──●──●──●──●──●──  (always deployable — the submodule's default branch)
        ╲      ╱  ╲    ╱
         ●──●─╱    ●──╱    ← short-lived feature branches (1-3 days)
```

This maps onto the OpenSpec change lifecycle, which is **spec-first and two-PR**: one change →
**a `spec/<change>` branch + spec PR** (proposal + delta specs + synced canonical specs; opened by
`/opsx:spec-pr`, merged by a human *before any code*) → then **a `feat/<change>` branch + code PR**
(implementation; opened by `/opsx:ship`, merged by a human). Both PRs live **inside the `platform/`
submodule**. **Never commit on `main`** — branch first; the agent never merges to `main` (a human
merges both PRs). The superproject gitlink bump (`cd <superproject> && git add platform && git commit`)
is a separate manual step.

- **Dev branches are costs.** Every day a branch lives, it accumulates merge risk.
- **Release branches are acceptable.** When you need to stabilize a release while main moves forward.
- **Feature flags > long branches.** Prefer deploying incomplete work behind flags rather than keeping it on a branch for weeks.

### 1. Commit Early, Commit Often

Each successful increment gets its own commit. Don't accumulate large uncommitted changes.

```
Work pattern:
  Implement slice → run toolchain gates → Commit → Next slice

Not this:
  Implement everything → Hope it works → Giant commit
```

Commits are save points. If the next change breaks something, you can revert to the last known-good state instantly. When implementing an OpenSpec change with `/opsx:ship-code`, tick each task in `tasks.md` and commit per work-unit (Red→Green→one commit).

### 2. Atomic Commits

Each commit does one logical thing:

```
# Good: Each commit is self-contained
git log --oneline
a1b2c3d feat(rag-core): add ACL filter to retrieve_kb query
d4e5f6g feat(bot-policy): add refuse_if no_citations to filter stage
h7i8j9k test(rag-core): cover cross-tenant isolation in retrieve_kb
m1n2o3p docs: update retrieve-kb-tool spec for the ACL change

# Bad: Everything mixed together
x1y2z3a add retrieval feature, fix ingestion, bump deps, refactor transport
```

### 3. Descriptive Messages — Conventional Commits

This repo uses **Conventional Commits**. Messages explain the *why*, not just the *what*, and **every commit ends with the Co-Authored-By trailer**:

```
# Good: Explains intent, Conventional Commit type, trailer
feat(rag-core): enforce server-side ACL inside retrieve_kb

Filter KB chunks by the inherited caller identity inside retrieve_kb so
the model can never widen its own access. Keeps the tenant_id-everywhere
and server-side-ACL invariants intact instead of post-filtering results.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>

# Bad: Describes what's obvious from the diff, no trailer
update retrieve_kb.py
```

**Format:**
```
<type>(<scope>): <short description>

<optional body explaining why, not what>

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
```

**Types:**
- `feat` — New feature
- `fix` — Bug fix
- `refactor` — Code change that neither fixes a bug nor adds a feature
- `test` — Adding or updating tests
- `docs` — Documentation only
- `chore` — Tooling, dependencies, config

Common scopes mirror the repo layout (packages/apps): `rag-core`, `agent-core`,
`ingest-core`, `llm-transport`, `bot-policy`, `backend`, `portal`, `worker-mello`,
`worker-mezon-go`, `kb-mcp`.

### 4. Keep Concerns Separate

Don't combine formatting changes with behavior changes. Don't combine refactors with features. Each type of change should be a separate commit — and ideally a separate PR / OpenSpec change:

```
# Good: Separate concerns
git commit -m "refactor(llm-transport): extract MiniMax client helper"
git commit -m "feat(rag-core): pin temperature 0 on synthesize/cite/filter"

# Bad: Mixed concerns
git commit -m "refactor transport and add temperature pinning"
```

**Separate refactoring from feature work.** A refactoring change and a feature change are two different changes — submit them separately. This makes each change easier to review, revert, and understand in history. Small cleanups (renaming a variable) can be included in a feature commit at reviewer discretion.

### 5. Size Your Changes

Target ~100 lines per commit/PR. Changes over ~1000 lines should be split.

```
~100 lines  → Easy to review, easy to revert
~300 lines  → Acceptable for a single logical change
~1000 lines → Split into smaller changes
```

A well-scoped OpenSpec change usually lands in this range. If a proposal's `tasks.md` implies a >1000-line diff, consider splitting it into multiple changes during `/opsx:propose`.

## Branching Strategy

### Feature Branches

```
main (always deployable — the submodule default branch)
  │
  ├── feat/retrieve-kb-acl       ← One OpenSpec change per branch
  ├── feat/agent-teams           ← Parallel work
  └── fix/citation-grounding     ← Bug fixes
```

- Branch from `main` (the submodule's default branch), inside the `platform/` submodule
- Keep branches short-lived (merge within 1-3 days) — long-lived branches are hidden costs
- Delete branches after merge
- Prefer feature flags over long-lived branches for incomplete features

### Branch Naming

```
feat/<change-name>          → feat/retrieve-kb-acl         (matches the OpenSpec change slug)
fix/<short-description>      → fix/citation-grounding
chore/<short-description>    → chore/bump-pyright
refactor/<short-description> → refactor/llm-transport
docs/<short-description>     → docs/openspec-workflow
```

## The Submodule Gitlink Bump

This is the load-bearing addition for this repo. **All the work above happens inside the `platform/` submodule.** Pointing the `funix-mezon-bot` superproject at a new `platform` commit is a **separate, manual step** — never auto-pushed by `/opsx:ship` or any automation.

```
Inside the submodule (this repo):
  feat/<change> → PR → review → merge to main → /opsx:archive

Then, manually, in the SUPERPROJECT (funix-mezon-bot):
  cd <funix-mezon-bot>
  git -C platform pull            # or check out the merged main commit
  git add platform                # stage the new gitlink (submodule SHA)
  git commit -m "chore: bump platform to <short-sha>"
  # push the superproject ONLY when a human decides to — never automated
```

Rules:
- Never commit a gitlink bump as part of submodule work — it lives in the superproject.
- Never auto-push the superproject. The gitlink bump is a deliberate, human-gated step.
- A bad gitlink bump is rolled back in the superproject (commit the previous `platform`
  SHA); the submodule history is untouched.

## Working with Worktrees

For parallel AI agent work, use git worktrees to run multiple branches simultaneously:

```bash
# Create a worktree for a feature branch (inside the submodule)
git worktree add ../meknow-retrieve-kb-acl feat/retrieve-kb-acl
git worktree add ../meknow-agent-teams feat/agent-teams

# Each worktree is a separate directory with its own branch
# When done, merge and clean up
git worktree remove ../meknow-retrieve-kb-acl
```

> Caveat for DB-backed work: DB-dependent Python tests need pgvector +
> `TEST_DATABASE_URL`. Two worktrees running migrations/tests against the same
> database will collide — point each at a separate test DB (and `alembic upgrade
> head` each) or serialize the runs. Note worktrees of a submodule have their own
> caveats; prefer separate clones if a worktree fights the submodule layout.

Benefits:
- Multiple agents can work on different changes simultaneously
- No branch switching needed (each directory has its own branch)
- If one experiment fails, delete the worktree — nothing is lost
- Changes are isolated until explicitly merged

### Automated Worktree Flow (`/opsx:ship --worktree`)

The `--worktree` flag on `/opsx:ship` automates the worktree lifecycle for a single
change's implementation. The PR is the final output — **switch to the next ticket**
after it's created. Post-merge tasks (archive etc.) are a separate workflow.

```bash
# Implementation runs in an isolated worktree; main checkout stays on `main`
/opsx:ship cNNNN-<change> --worktree
```

What happens:

1. **Preflight** validates the handoff, spec contract, and toolchain in the main checkout.
2. **Worktree** is created automatically (`isolation: 'worktree'` on the agent call).
3. Inside the worktree: `git switch -c feat/<change> main` → all phases
   (Implement → Verify → Review → Evidence → Sync → Changelog → chore commit).
4. **Stops after the chore commit** — no push, no PR. Test locally.
5. **Worktree is cleaned up** after the agent returns.
6. Branch `feat/<change>` persists in shared refs.
7. Run `/opsx:ship-pr <change>` to archive + push + create the PR.

**Parallel workflow example:**

```bash
# Touch point 1-2: spec phase for change A
/opsx:propose "change-a"
/opsx:spec "change-a"               # human reviews locally → APPROVE
/opsx:spec-pr "change-a"            # human reviews spec PR remotely → merges

# Touch point 3: ship — AI implements, stops for local test
/opsx:ship "change-a" --worktree

# Test locally, then archive + push + PR
/opsx:ship-pr "change-a"

# Switch to change B while waiting for PR review
/opsx:propose "change-b"
/opsx:spec "change-b"

# Touch point 4: human reviews change A's PR → merges (archive lands on main)
```

> **Caveat for DB-backed tests:** Two worktrees running DB-dependent pytest suites
> against the same `TEST_DATABASE_URL` can collide. Mitigations: serialize DB changes,
> use separate test DBs, or note that DB tests skip gracefully without pgvector.

### Worktree Lifecycle

| Step | Action | Responsible |
|---|---|---|
| Create | Runtime creates via `agent({isolation:'worktree'})` | Automatic |
| Feature branch | `git switch -c feat/<change> main` inside worktree | Agent |
| Implementation | All ship-code phases (implement → changelog) | Agent |
| Cleanup | Worktree removed after agent returns | Runtime |
| Local test | Inspect branch, run tests | **You** (touch point 3) |
| Archive + PR | `/opsx:ship-pr <change>` → archive + push + gh pr create | Agent |
| PR review | Remote code review → merge (archive lands on main) | **You** (touch point 4) |

No manual worktree cleanup is needed — the workflow runtime handles it.

## The Save Point Pattern

```
Agent starts work
    │
    ├── Makes a change
    │   ├── toolchain gates pass? → Commit → Continue
    │   └── gates fail?           → Revert to last commit → Investigate
    │
    └── Change complete → All commits form a clean history
```

This pattern means you never lose more than one increment of work. If an agent goes off the rails, `git reset --hard HEAD` takes you back to the last successful state.

## Change Summaries

After any modification, provide a structured summary. This makes review easier, documents scope discipline, and surfaces unintended changes:

```
CHANGES MADE:
- packages/rag-core/retrieve_kb.py: ACL filter now applied inside the query
- packages/rag-core/tests/test_retrieve_kb.py: cross-tenant isolation case

THINGS I DIDN'T TOUCH (intentionally):
- packages/ingest-core: chunking is unaffected by this change
- apps/portal: no API surface change, so generated types stay put

POTENTIAL CONCERNS:
- Cache keys must include the ACL-cohort hash — confirm they weren't widened.
- Needs an alembic migration? No — the index already exists.
```

The "DIDN'T TOUCH" section shows you exercised scope discipline and didn't go on an unsolicited renovation.

## Pre-Commit Hygiene

Before every commit, run the **owning toolchain's** gates for what you touched:

```bash
# 1. Check what you're about to commit
git diff --staged

# 2. Ensure no secrets (never commit the LLM key or any credential)
git diff --staged | grep -iE "password|secret|api_key|token|MINIMAX_API_KEY|DATABASE_URL"

# 3. Run the gates for the touched toolchain(s):
#    Python member (uv)
uv --directory <member> run ruff check . && uv --directory <member> run ruff format --check . && \
  uv --directory <member> run pyright && uv --directory <member> run python -m pytest -q
#    Go module
go build ./... && go vet ./... && go test -race ./...
#    Portal
pnpm typecheck && pnpm lint && pnpm test

# 4. Always validate the change spec
openspec validate "<change>" --strict
```

If you want a local guard, a simple `.git/hooks/pre-commit` that runs the touched toolchain's lint + format check (`ruff check`/`ruff format --check`, or `gofmt -l`, or `pnpm lint`) is sufficient.

## Handling Generated Files

- **Commit** dependency manifests (`pyproject.toml`/`uv.lock`, `go.mod`/`go.sum`,
  `apps/portal/package.json`/`pnpm-lock.yaml`), alembic migrations under
  `apps/backend/migrations/`, and OpenSpec artifacts under `openspec/`.
- **Commit generated OpenAPI types** for the portal — but regenerate them from the
  backend spec, never hand-edit (the OpenAPI-is-the-contract invariant).
- **Don't commit** build output (`dist/`, `__pycache__/`, `.pytest_cache/`,
  `node_modules/`, Go binaries, `cover.out`), local config, or any `.env` with real
  secrets.
- **`.gitignore`** should cover at least: `dist/`, `__pycache__/`, `node_modules/`,
  `*.env`, `cover.out`, and editor cruft.

## Using Git for Debugging

```bash
# Find which commit introduced a bug (run the owning toolchain's gate at each midpoint)
git bisect start
git bisect bad HEAD
git bisect good <known-good-commit>
git bisect run uv --directory packages/rag-core run python -m pytest -q -k TestName

# View what changed recently
git log --oneline -20
git diff HEAD~5..HEAD -- packages/rag-core/

# Find who last changed a specific line
git blame packages/rag-core/retrieve_kb.py

# Search commit messages for a keyword
git log --grep="tenant" --oneline
```

## CHANGELOG Discipline

The repo keeps a root `CHANGELOG.md` in **Keep a Changelog** format — one bullet per shipped change. **`/opsx:ship` generates the entry for you** when shipping an approved change; you rarely hand-edit it. The shape:

```markdown
# Changelog

## [Unreleased]
### Added
- retrieve_kb now enforces ACL server-side using the inherited caller identity.

### Fixed
- Cross-tenant leak where a cache key omitted tenant_id.
```

## Opening a PR

This repo uses the `gh` CLI, run **inside the submodule**. `/opsx:ship` runs this for you; to do it by hand:

```bash
gh pr create --base main --title "feat(rag-core): server-side ACL in retrieve_kb" --body "$(cat <<'EOF'
## Summary
- ...

## Test plan
- uv --directory packages/rag-core run ruff check . && pyright && pytest -q
- bash benchmarks/gates/tenant-isolation-test.sh

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

`/opsx:ship` **stops at PR opened — it does not auto-merge.** Archiving the change with `/opsx:archive` is a separate, post-merge human step. The superproject gitlink bump is yet another separate manual step (see "The Submodule Gitlink Bump").

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll commit when the feature is done" | One giant commit is impossible to review, debug, or revert. Commit each slice. |
| "The message doesn't matter" | Messages are documentation. Use Conventional Commits + the Co-Authored-By trailer. |
| "I'll squash it all later" | Squashing destroys the development narrative. Prefer clean incremental commits. |
| "Branches add overhead" | Short-lived branches are free. Long-lived branches are the problem — merge within 1-3 days. |
| "I'll just commit on main" | Don't. Branch first (`feat/<change>`); `main` stays deployable. |
| "I'll bump the superproject gitlink in the same flow" | No. The gitlink bump is a separate, manual, human-gated step in the superproject. |
| "I'll split this change later" | Large changes are harder to review, riskier to deploy, and harder to revert. Split before submitting. |

## Red Flags

- Large uncommitted changes accumulating
- Commit messages like "fix", "update", "misc" — or missing the Co-Authored-By trailer
- Commits made directly on `main`
- Formatting changes mixed with behavior changes
- Committing build output (`dist/`, `__pycache__/`, `node_modules/`) or a `.env` with a real `MINIMAX_API_KEY` / `DATABASE_URL`
- A gitlink bump committed inside the submodule (it belongs in the superproject)
- An automated push of the superproject gitlink bump
- Long-lived branches that diverge significantly from main
- Force-pushing to shared branches

## Project notes

- One OpenSpec change → a `spec/<change>` branch + SPEC PR (the contract), then a
  `feat/<change>` branch + CODE PR, **inside the `platform/` submodule**. Drive
  non-trivial work through `/opsx:propose` → `/opsx:spec` → `/opsx:spec-pr` →
  `/opsx:ship` → `/opsx:address-review` → `/opsx:archive`.
- **`/opsx:ship`** is the autonomous lane: ship-plan (group into work-units) →
  ship-code (each unit Red→Green→one commit) → verify (the resolver-selected
  per-toolchain gates + `openspec validate "<change>" --strict`) → reconcile delta vs
  canonical (drift → stop) → prepend the `CHANGELOG.md` entry → commit (with the
  Co-Authored-By trailer) → push →
  open the PR via `gh`, then **STOPS at the PR** (no auto-merge). `--dry-run` stops
  before push/PR.
- `/opsx:archive` runs **after** the PR merges, moving the change to
  `openspec/changes/archive/YYYY-MM-DD-<change>/`.
- **The superproject gitlink bump** (`cd <funix-mezon-bot> && git add platform && git
  commit`) is a separate, manual step that is never automated or auto-pushed.
- Capture verification evidence under `openspec/changes/<name>/evidence/`.
- See the sibling `ci-cd-and-automation` skill for the CI gates (the four-job
  `rag-engine.yml`, checked out `submodules: recursive`) these commits must pass, and
  `documentation-and-adrs` for when a change needs an ADR / spec update.

## Verification

For every commit:

- [ ] Commit does one logical thing
- [ ] Message uses a Conventional Commit type and explains the why
- [ ] Message ends with the `Co-Authored-By: Claude Opus 4.8 (1M context)` trailer
- [ ] On a `feat/`/`fix/` branch inside the submodule, not on `main`
- [ ] The touched toolchain's gates pass before committing (ruff/pyright/pytest, or go vet/test -race, or pnpm typecheck/lint/test)
- [ ] No secrets in the diff
- [ ] No formatting-only changes mixed with behavior changes
- [ ] No gitlink bump committed inside the submodule (that belongs in the superproject, manually)
- [ ] `.gitignore` covers `dist/`, `__pycache__/`, `node_modules/`, `.env`
