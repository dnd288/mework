---
name: using-agent-skills
description: Discovers and invokes agent skills (MeKnow-adapted). Use when starting a session or when you need to discover which skill applies to the current task. This is the meta-skill that governs how all other skills are discovered and invoked, and it routes work onto this repo's mzspec-driven OpenSpec lifecycle (/opsx:propose → /opsx:spec → /opsx:spec-pr → /opsx:ship [ship-plan → ship-code] → /opsx:address-review → /opsx:archive).
tags: []
---

# Using Agent Skills

## Overview

Agent Skills is a collection of engineering workflow skills organized by development phase. Each skill encodes a specific process that senior engineers follow. This meta-skill helps you discover and apply the right skill for your current task.

In the Mezon Mentor Bot ("MeKnow") platform, the lifecycle is anchored to **OpenSpec** driven by the **mzspec** `/opsx:*` workflow (the core spine; these skills are the "how to build well" extension over it): non-trivial work starts as a change proposal, not code. The routing table below maps each development phase to the matching skill(s) *and* the `/opsx:*` command that drives it. Alongside the engineering-practice skills, the repo also carries **frontend** skills for the `apps/portal` React 19 + Vite surface — route to those when the task is portal-UI shaped rather than backend/worker logic. (Converge orchestration is **retired** — see `.converge/DEPRECATED.md`; do not route new work to it.)

## Skill Discovery

When a task arrives, identify the development phase and apply the corresponding skill. The `/opsx:*` column shows where the OpenSpec workflow plugs in.

```
Task arrives
    │
    ├── Don't know what you want yet? ──────→ /opsx:explore  (think-only mode, no code)
    ├── New project/feature/change? ────────→ /opsx:propose  + spec-driven-development
    │                                          (writes proposal.md + delta specs)
    ├── Reviewing/revising a spec? ──────────→ /opsx:spec  + spec-review-and-quality
    │                                          (6-axis cross-validate → revise until APPROVE)
    ├── Spec approved — merge the contract? ─→ /opsx:spec-pr  (sync delta→canonical + SPEC PR;
    │                                          a human merges BEFORE any code)
    ├── Have a proposal, need a design? ─────→ design.md      + planning-and-task-breakdown
    │                                          (decisions, dependency graph, tasks.md)
    ├── Writing/running tests first (Red)? ──→ test-driven-development
    ├── Implementing code (Green)? ──────────→ incremental-implementation + /opsx:ship
    │                                          (test-first units; tick tasks.md as you go)
    ├── Something broke? ────────────────────→ debugging-and-error-recovery
    ├── Reviewing code? ─────────────────────→ code-review-and-quality
    │   ├── Security concerns? ──────────────→ security-and-hardening
    │   └── Too complex? ────────────────────→ code-simplification
    ├── Committing/branching/versioning? ────→ git-workflow-and-versioning
    ├── CI/CD pipeline work? ────────────────→ ci-cd-and-automation
    ├── Writing docs/ADRs? ──────────────────→ documentation-and-adrs
    ├── Implementing a MERGED spec? ─────────→ /opsx:ship  (remote default: implement → verify →
    │                                          review → reconcile → open the CODE PR) — needs the spec PR merged
    ├── Responding to PR review feedback? ───→ /opsx:address-review
    └── Code PR merged? ─────────────────────→ /opsx:archive
```

> The workflow is **spec-first, two-PR, remote-PR-gated**: `/opsx:spec` → `/opsx:spec-pr` (human
> merges the spec) precedes `/opsx:ship` (human merges the code PR). The agent never merges to
> `main`. See `docs/openspec-workflow.md`.

## Core Operating Behaviors

These behaviors apply at all times, across all skills. They are non-negotiable.

### 1. Surface Assumptions

Before implementing anything non-trivial, explicitly state your assumptions:

```
ASSUMPTIONS I'M MAKING:
1. [assumption about requirements]
2. [assumption about architecture]
3. [assumption about scope]
→ Correct me now or I'll proceed with these.
```

Don't silently fill in ambiguous requirements. The most common failure mode is making wrong assumptions and running with them unchecked. Surface uncertainty early — it's cheaper than rework. In this repo, capture these in the change's `proposal.md` so the human reviews them before `/opsx:ship`.

### 2. Manage Confusion Actively

When you encounter inconsistencies, conflicting requirements, or unclear specifications:

1. **STOP.** Do not proceed with a guess.
2. Name the specific confusion.
3. Present the tradeoff or ask the clarifying question.
4. Wait for resolution before continuing.

**Bad:** Silently picking one interpretation and hoping it's right.
**Good:** "I see X in the `openspec/specs/` baseline but Y in the existing code. Which takes precedence?"

### 3. Push Back When Warranted

You are not a yes-machine. When an approach has clear problems:

- Point out the issue directly
- Explain the concrete downside (quantify when possible — "this adds ~200ms to p95 latency" not "this might be slower")
- Propose an alternative
- Accept the human's decision if they override with full information

Sycophancy is a failure mode. "Of course!" followed by implementing a bad idea helps no one. Honest technical disagreement is more valuable than false agreement.

### 4. Enforce Simplicity

Your natural tendency is to overcomplicate. Actively resist it.

Before finishing any implementation, ask:
- Can this be done in fewer lines?
- Are these abstractions earning their complexity?
- Would a staff engineer look at this and say "why didn't you just..."?

If you build 1000 lines and 100 would suffice, you have failed. Prefer the boring, obvious solution. Cleverness is expensive. This repo's hard invariants encode the same instinct: a new bot/capability is a new `BotPolicy` row, not new code; a new engine capability is a lobe/path registry row, never an interpreter branch.

### 5. Maintain Scope Discipline

Touch only what you're asked to touch.

Do NOT:
- Remove comments you don't understand
- "Clean up" code orthogonal to the task
- Refactor adjacent systems as a side effect
- Delete code that seems unused without explicit approval
- Add features not in the change's `proposal.md`/`tasks.md` because they "seem useful"

Your job is surgical precision, not unsolicited renovation.

### 6. Verify, Don't Assume (Evidence Required)

Every skill includes a verification step. A task is not complete until verification passes. "Seems right" is never sufficient — there must be evidence from the **resolver gates**: `uv run ruff`/`pyright`/`pytest` for Python members, `go build/vet/test -race` (go 1.24) for Go modules, `pnpm typecheck/lint/test` for `apps/portal`, `bash benchmarks/ci-free-gates.sh` for the benchmark ladder, plus `openspec validate "<change>" --strict` on every change.

Capture that evidence under `openspec/changes/<name>/evidence/` so reviewers can see proof — command output, test logs, gate results — not just claims.

## Failure Modes to Avoid

These are the subtle errors that look like productivity but create problems:

1. Making wrong assumptions without checking
2. Not managing your own confusion — plowing ahead when lost
3. Not surfacing inconsistencies you notice
4. Not presenting tradeoffs on non-obvious decisions
5. Being sycophantic ("Of course!") to approaches with clear problems
6. Overcomplicating code and APIs
7. Modifying code or comments orthogonal to the task
8. Removing things you don't fully understand
9. Building without a proposal because "it's obvious"
10. Skipping verification (or not saving evidence) because "it looks right"

## Skill Rules

1. **Check for an applicable skill before starting work.** Skills encode processes that prevent common mistakes.

2. **Skills are workflows, not suggestions.** Follow the steps in order. Don't skip verification steps.

3. **Multiple skills can apply.** A feature might involve `spec-driven-development` (`/opsx:propose`) → `spec-review-and-quality` (`/opsx:spec` → `/opsx:spec-pr`) → `planning-and-task-breakdown` (design.md + tasks.md) → `test-driven-development` → `incremental-implementation` (`/opsx:ship-code`) → `code-review-and-quality` → `code-simplification` → `git-workflow-and-versioning` (`/opsx:ship`) in sequence.

4. **When in doubt, start with a proposal.** If the task is non-trivial and there's no OpenSpec change, begin with `spec-driven-development` and run `/opsx:propose`.

## Lifecycle Sequence (MeKnow / OpenSpec)

For a complete change, the typical sequence is:

```
1.  /opsx:explore               → Think through the idea (no code)
2.  spec-driven-development     → /opsx:propose: proposal.md + delta specs
2b. spec-review-and-quality     → /opsx:spec: cross-validate 6 axes → revise until clean
2c. spec-review-and-quality     → /opsx:spec-pr: sync delta→canonical + open the SPEC PR
                                   (a human merges the contract BEFORE any code)
3.  planning-and-task-breakdown → design.md (decisions) + tasks.md (ordered tasks)
4.  test-driven-development     → Red: failing test first for each slice
5.  incremental-implementation  → Green: /opsx:ship-code, build unit by unit, tick tasks.md
6.  debugging-and-error-recovery→ Reproduce → localize → fix → guard when something breaks
7.  code-review-and-quality     → Review before merge
8.  security-and-hardening      → Multi-tenant isolation, server-side ACL, secrets handling
9.  code-simplification         → Reduce complexity while preserving behavior
10. git-workflow-and-versioning → Clean commits / branch
11. documentation-and-adrs      → Document the why (change design.md + docs/)
12. /opsx:ship                  → ship-plan → ship-code (Red→Green) → verify (resolver gates)
                                   → reconcile → changelog → open the CODE PR
13. /opsx:address-review        → Respond to PR feedback
14. /opsx:archive               → After PR merges, move change to openspec/changes/archive/
```

Not every task needs every skill. A bug fix might only need: `debugging-and-error-recovery` → `test-driven-development` → `code-review-and-quality`.

## Quick Reference

| Phase | Skill / Command | One-Line Summary |
|-------|-----------------|------------------|
| Explore | `/opsx:explore` | Think through an idea, clarify requirements — no code written |
| Define | spec-driven-development + `/opsx:propose` | Proposal and delta specs before code |
| Review spec | spec-review-and-quality + `/opsx:spec` | 6-axis cross-validate → revise until clean, minimal, testable, complete |
| Merge spec | spec-review-and-quality + `/opsx:spec-pr` | Sync delta→canonical, open the code-free SPEC PR (human merges before any code) |
| Plan | planning-and-task-breakdown (design.md + tasks.md) | Decompose into small, verifiable tasks |
| Verify (Red) | test-driven-development | Failing test first, then make it pass |
| Build (Green) | incremental-implementation + `/opsx:ship-code` | Test-first units, Red→Green→one commit, tick tasks.md, verify each |
| Verify | debugging-and-error-recovery | Reproduce → localize → fix → guard |
| Review | code-review-and-quality | Five-axis review with quality gates |
| Review | security-and-hardening | Multi-tenant isolation, server-side ACL, Fernet-encrypted secrets |
| Review | code-simplification | Preserve behavior while reducing complexity |
| Ship | git-workflow-and-versioning | Atomic commits, clean history |
| Ship | ci-cd-and-automation | Automated resolver gates (ruff/pyright/pytest, go test -race, pnpm, benchmarks) |
| Ship | documentation-and-adrs | Document the why, not just the what |
| Ship | `/opsx:ship` | Autonomous ship-plan → ship-code (Red→Green) → verify → reconcile → changelog → CODE PR |
| Ship | `/opsx:address-review` | Respond to PR review feedback |
| Ship | `/opsx:archive` | Finalize change after PR merges |

## MeKnow notes

- The router's "spec" node is OpenSpec: never hand-write a competing PRD format —
  run `/opsx:propose` and let it generate `proposal.md`, delta specs, `design.md`,
  and `tasks.md`. Read the relevant `openspec/specs/<capability>/spec.md` baseline
  (e.g. `data-model`, `bot-policy`, `ingestion-pipeline`, `retrieve-kb-tool`,
  `retrieval-runtime`, `auth-and-tenancy`, `api-surface`, `mcp-and-skills`) before
  changing a subsystem. Original design rationale lives in `docs/design/NNNN-*.md`
  (the former RFCs).
- **Evidence lives at `openspec/changes/<name>/evidence/`.** Rule 6 (verify, don't
  assume) is satisfied by the **gate resolver** output — the resolver maps each
  touched path to its owning package and runs that toolchain's gates: `uv run
  ruff`/`pyright`/`pytest` (Python `uv` workspace), `go build/vet/test -race`
  (go 1.24 modules), `pnpm typecheck/lint/test` (`apps/portal`), and
  `bash benchmarks/ci-free-gates.sh` — plus `openspec validate "<change>" --strict`
  always.
- Honor the repo's hard invariants while routing any task: **no LangChain/LangGraph**
  (`anthropic-sdk-python` directly); **citations mandatory** (`refuse_if:
  no_citations`); **`temperature == 0`** on `synthesize`/`cite`/`filter`; **ACL is
  server-side** inside `retrieve_kb` (caller identity inherited, never trusted from
  the model); **multi-tenant by default** (`tenant_id` on every table/query/cache
  key; cross-tenant joins are bugs); **versions are append-only**; the **compression
  invariant** (raw KB chunks never leave a sub-agent); **OpenAPI is the API contract**
  (portal types generated, never hand-written).
- Components you'll route work across: `apps/backend` (API + OpenAPI), the workers
  (`worker-ingest`, `worker-retrieve`, `worker-task`, `worker-webhook`, …), and the
  shared `packages/` (`rag-core`, `agent-core`, `arag-core`, `ingest-core`,
  `llm-transport`). This repo is consumed as the `platform/` submodule of
  `funix-mezon-bot`; all OpenSpec work happens inside this submodule.
- The `/opsx:ship` pipeline is the autonomous version of the Ship phase and only
  runs against a change whose spec is already **approved** by a human.
