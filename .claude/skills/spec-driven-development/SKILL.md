---
name: spec-driven-development
description: Creates specs before coding (MeKnow-adapted onto OpenSpec). Use when starting a new feature or significant change and no OpenSpec change exists yet. Use when requirements are unclear, ambiguous, or only exist as a vague idea. Maps SPECIFY/PLAN/TASKS/IMPLEMENT onto /opsx:propose, design.md, tasks.md, and /opsx:ship (ship-plan → ship-code) — former RFCs now live as capability specs (rationale in docs/design/); do not invent a competing PRD file.
tags: []
---

# Spec-Driven Development

## Overview

Write a structured specification before writing any code. The spec is the shared source of truth between you and the human engineer — it defines what we're building, why, and how we'll know it's done. Code without a spec is guessing.

**This repo (Mezon Mentor Bot / "MeKnow") already implements spec-driven development via [OpenSpec](https://github.com/Fission-AI/OpenSpec).** Don't author a freeform PRD file. This repo **replaced its former RFCs with OpenSpec capability specs**: the canonical record is `openspec/specs/<capability>/spec.md`, and the original RFC prose now lives as rationale in `docs/design/NNNN-<slug>.md`, linked from each capability's `## Purpose`. A `docs/prd.md` exists as background only — **specs are canonical**, do not create a competing PRD. The artifacts are generated and tracked by the `/opsx:*` workflow under `openspec/changes/cNNNN-<slug>/`. This skill explains *what to put in those artifacts* — it doesn't replace them.

## When to Use

- Starting a new feature or capability
- Requirements are ambiguous or incomplete
- The change touches multiple files, packages, or toolchains
- You're about to make an architectural decision
- The task would take more than 30 minutes to implement

**When NOT to use:** Single-line fixes, typo corrections, or changes where requirements are unambiguous and self-contained.

## The Gated Workflow (mapped onto OpenSpec)

Spec-driven development has four phases. Do not advance to the next phase until the current one is validated by a human. Each phase corresponds to an OpenSpec step:

```
SPECIFY ────────→ PLAN ─────────────→ TASKS ────────→ IMPLEMENT
/opsx:propose     design.md          tasks.md        /opsx:ship
(proposal.md +    (decisions &       (ordered,       (ship-plan →
 delta specs)      rationale)         verifiable)      ship-code)
                   ui.md
                   (UI/visual
                    design)
   │                 │                  │                 │
   ▼                 ▼                  ▼                 ▼
 Human            Human              Human             Human
 reviews          reviews            reviews           reviews
```

`/opsx:explore` is the optional pre-SPECIFY think-only mode for shaping a vague idea before you propose.

### Phase 1: Specify → `/opsx:propose`, then quality-assure → `/opsx:spec`

Start with a high-level vision. Ask the human clarifying questions until requirements are concrete, then run `/opsx:propose "<name>"`, which generates `proposal.md` and the delta specs under `openspec/changes/cNNNN-<slug>/specs/`.

`/opsx:propose` is a **single-pass draft** — it generates artifacts but does not judge their quality. Before planning or implementing, run **`/opsx:spec <name>`** to cross-validate the draft across the six spec-review axes (Structure/validity · Clarity/KISS · Testability · Minimality/YAGNI · Consistency/DRY · Completeness) and revise it until clean. The quality bar is the `spec-review-and-quality` skill; the six coverage areas below are *what* to cover, those axes are *how well*. A clean spec is what lets `/opsx:ship` produce a complete feature, not a partial one.

**Surface assumptions immediately.** Before writing proposal content, list what you're assuming, in `proposal.md`:

```
ASSUMPTIONS I'M MAKING:
1. This change affects the retrieve-kb-tool capability (not retrieval-runtime)
2. The new behavior is a new BotPolicy/BotVersion row + prompt file — no interpreter code
3. ACL stays server-side inside retrieve_kb; caller identity is inherited, never model-supplied
4. Every table/query/cache key carries tenant_id; versions stay append-only
→ Correct me now or I'll proceed with these.
```

Don't silently fill in ambiguous requirements. The proposal's entire purpose is to surface misunderstandings *before* code gets written — assumptions are the most dangerous form of misunderstanding.

**The proposal + delta specs must cover these six core areas** (use this as a review checklist):

1. **Objective** — What are we building and why? Who is the actor (asker in Mezon chat, tenant admin, worker, portal user)? What does success look like? Which `openspec/specs/<capability>` baseline does it extend or change, and which `docs/design/NNNN-*.md` rationale backs it?

2. **Commands** — Full executable commands with flags, not just tool names. State the touched toolchain(s); the **gate resolver** (`.claude/workflows/lib/gate-resolver.js`) maps each path to its owning package and runs that toolchain's gates:
   ```
   Python (member dir D):  uv --directory D run ruff check .
                           uv --directory D run ruff format --check .
                           uv --directory D run pyright
                           uv --directory D run python -m pytest -q
   Go (module dir M):      go build ./...   go vet ./...   go test -race ./...
   TS (apps/portal):       pnpm typecheck   pnpm lint   pnpm test   (pnpm test:e2e — gated)
   Benchmarks:             bash benchmarks/ci-free-gates.sh
   Always:                 openspec validate "<change>" --strict
   ```

3. **Project Structure** — Where the code lives. Name the concrete packages the change touches and their toolchain, e.g.:
   ```
   packages/rag-core/        → retrieve_kb, chunking, ACL scoping (py)
   packages/agent-core/      → retrieval runtime: synthesize/cite/filter stages (py)
   packages/bot-policy/      → declarative BotPolicy interpreter (py)
   apps/backend/             → OpenAPI surface; migrations under apps/backend/migrations (py)
   apps/kb-mcp/              → kb.* MCP tools (embedded transport by default) (py)
   apps/worker-*/            → ingest/retrieve/task workers (py); worker-mello/-mezon-go (go)
   apps/portal/              → React 19 portal; types generated from OpenAPI (ts)
   benchmarks/gates/         → deterministic gate scripts backing spec scenarios
   openspec/changes/<name>/  → this change's proposal, specs, design, tasks, evidence
   docs/design/NNNN-*.md     → original RFC rationale, linked from the capability Purpose
   ```

4. **Code Style** — One real snippet showing the convention beats three paragraphs describing it. Match the package's language. Python (no LangChain — `anthropic-sdk-python` directly; tenant_id threaded everywhere):
   ```python
   def retrieve_kb(ctx: RequestContext, query: str) -> list[Memo]:
       # ACL is server-side; caller identity is inherited, never model-supplied.
       acl = ctx.acl_cohort  # derived from authenticated tenant + ACL
       chunks = store.search(tenant_id=ctx.tenant_id, acl_cohort=acl, query=query)
       return [memo_from(chunk) for chunk in chunks]  # only memo-shaped objects cross the boundary
   ```
   Go (standalone module, wrapped errors): `fmt.Errorf("...: %w", err)`, table-driven tests, `go test -race`. TS (portal): generated OpenAPI types, never hand-written.

5. **Testing Strategy** — State the level and the resolver-selected gates. Python: `pytest -q` (unit), coverage `pytest -q --cov --cov-report=term-missing`. Go: `go test -race ./...`. TS: Vitest (`pnpm test`), Playwright (`pnpm test:e2e`, gated tier). Cross-cutting behavioral guarantees are proven by the **benchmark gates** (`benchmarks/gates/`): faithfulness >= 0.85, citation accuracy >= 0.95, p95 latency <= 12s, cross-tenant isolation, ACL escape. Reference the backing gate script from any `#### Scenario:` that asserts a guarantee; the golden set is `tests/fixtures/golden_set.json`.

6. **Boundaries** — Three-tier system:
   - **Always do:** Run the resolver-selected gates + `openspec validate --strict` before commits; thread `tenant_id` through every table/query/cache key/log line; keep ACL server-side in `retrieve_kb`; `temperature == 0` on synthesize/cite/filter; emit citations or refuse (`refuse_if: no_citations`); only memo-shaped objects cross stage boundaries.
   - **Ask first:** New Alembic migration, new dependency, new capability spec, changes to the OpenAPI or `kb.*` MCP contract, new benchmark gate, CI config changes.
   - **Never do:** Commit secrets; introduce LangChain/LangGraph/Haystack/LlamaIndex; emit an ungrounded answer; mutate a `BotVersion`/`KBVersion` in place (versions are append-only); do a cross-tenant join; add an interpreter branch where a registry row / new `BotPolicy` row belongs.

**Do not write a separate spec template file.** `/opsx:propose` produces `proposal.md` and delta specs in the OpenSpec scenario format. Fill those; the six areas above are your coverage checklist for them. Do not create a competing `docs/prd.md`.

**Reframe instructions as success criteria.** When receiving vague requirements, translate them into concrete, testable conditions in `proposal.md`:

```
REQUIREMENT: "Make answers more trustworthy"

REFRAMED SUCCESS CRITERIA:
- filter stage refuses (refuse_if: no_citations) rather than emit ungrounded claims
- Faithfulness gate >= 0.85 and citation-accuracy gate >= 0.95 on the golden set
- Cross-tenant isolation test passes (no leakage across tenant_id)
→ Are these the right targets?
```

### Phase 2: Plan → `design.md` (and `ui.md` for UI changes)

With the validated proposal, capture the technical plan in the change's `design.md`
and, for user-facing changes, the visual plan in `ui.md`:

1. Identify the major components and their dependencies (which packages, in what order, and each unit's toolchain — py/go/ts)
2. Determine the implementation order (foundations first — e.g. data model/migration before the API surface)
3. Note risks and mitigation strategies (tenant isolation, ACL scoping, citation refusal)
4. Identify what can be built in parallel vs. sequential
5. Define verification checkpoints between phases
6. For UI changes: create `ui.md` with wireframes, component tree, UI states,
   user flows, and visual decisions following the `ui-design` skill

`design.md` should be reviewable: the human reads it and says "yes, that's the right approach" or "no, change X." See `planning-and-task-breakdown` for the design and dependency-graph mechanics.

### Phase 3: Tasks → `tasks.md`

Break the plan into discrete, implementable tasks in `tasks.md` (generated by `/opsx:propose`, refined as needed):

- Each task is completable in a single focused session
- Each task names its toolchain and has explicit acceptance criteria
- Each task includes a verification step (resolver-selected gates, benchmark gates, manual check)
- Tasks are ordered by dependency, not perceived importance
- No task should require changing more than ~5 files or spanning more than one toolchain

`tasks.md` uses OpenSpec checkboxes; tick them off during `/opsx:ship-code`. The task structure and sizing rules live in `planning-and-task-breakdown` — follow that skill rather than duplicating it here.

### Phase 4: Implement → `/opsx:ship` (ship-plan → ship-code)

The spec contract is merged first via `/opsx:spec-pr` (a human merges the SPEC PR so the contract is law on `main`). Then `/opsx:ship` runs the whole pipeline autonomously: `ship-plan` groups the change into test-first work-units, and `ship-code` executes each unit Red→Green→one commit, ticking `tasks.md` as it goes. Follow `incremental-implementation` (thin vertical slices) and `test-driven-development` (failing test first). Ship continues: verify (resolver-selected gates + benchmark gates + `openspec validate --strict`) → reconcile delta vs canonical (drift → stop) → changelog → commit → push → open the CODE PR.

## Keeping the Spec Alive

The spec is a living document, not a one-time artifact:

- **Update when decisions change** — If the data model needs to change, update the change's delta specs / `design.md` first, then implement.
- **Update when scope changes** — Features added or cut should be reflected in `proposal.md` and `tasks.md`.
- **Commit the change** — The whole `openspec/changes/<name>/` tree belongs in version control alongside the code. (All OpenSpec work happens inside this `platform/` submodule.)
- **Sync and archive** — Run `/opsx:sync` to merge delta specs into `openspec/specs/`, and `/opsx:archive` after the PR merges so the baseline always reflects reality.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "This is simple, I don't need a proposal" | Simple tasks don't need *long* proposals, but they still need acceptance criteria. A short proposal is fine. |
| "I'll write the spec after I code it" | That's documentation, not specification. The proposal's value is forcing clarity *before* code. |
| "The spec will slow us down" | A 15-minute proposal prevents hours of rework. |
| "Requirements will change anyway" | That's why OpenSpec changes are living documents. An outdated proposal still beats none. |
| "I'll just write my own PRD" | Don't — it competes with OpenSpec. `docs/prd.md` is background; specs are canonical. Use `/opsx:propose`. |

## Red Flags

- Starting to write code without an OpenSpec change
- Asking "should I just start building?" before clarifying what "done" means
- Implementing features not mentioned in `proposal.md` or `tasks.md`
- Making architectural decisions without recording them in `design.md` (or its `docs/design/` rationale)
- Hand-writing a PRD file that duplicates or conflicts with OpenSpec specs

## Verification

Before proceeding to implementation, confirm:

- [ ] The proposal + delta specs cover all six core areas (Objective, Commands, Project Structure, Code Style, Testing Strategy, Boundaries)
- [ ] `/opsx:spec` ran and the spec review verdict is **approve** (no Blocker/Required findings across the six axes)
- [ ] The human has reviewed and approved the change
- [ ] Success criteria are specific and testable (and reference a backing benchmark gate where they assert a guarantee)
- [ ] Boundaries (Always/Ask First/Never) are defined
- [ ] `design.md` and `tasks.md` exist and `openspec validate "<change>" --strict` passes
- [ ] `ui.md` exists and is well-formed for user-facing changes

## MeKnow notes

- This skill is the SPECIFY/PLAN/TASKS/IMPLEMENT mapping onto OpenSpec: SPECIFY =
  `/opsx:propose` (proposal.md + delta specs), PLAN = `design.md` + `ui.md` (for UI), TASKS =
  `tasks.md`, IMPLEMENT = `/opsx:ship` (ship-plan → ship-code, once the spec PR is
  merged). Never
  introduce a parallel PRD format — the former RFCs are now OpenSpec capability
  specs, with their rationale preserved in `docs/design/NNNN-*.md` (linked from each
  capability's Purpose). `docs/prd.md` is background, not canonical.
- Read the relevant baseline at `openspec/specs/<capability>/spec.md` before
  proposing — it is the canonical record of already-implemented behavior.
- Bake the repo invariants into Boundaries: `tenant_id` everywhere (every table,
  query, cache key, log line; no cross-tenant joins); citations mandatory
  (`refuse_if: no_citations`); `temperature == 0` on synthesize/cite/filter; ACL
  enforced **server-side** in `retrieve_kb`; versions **append-only**
  (`BotVersion`/`KBVersion`); MCP is the internal tool boundary (no LangChain);
  only memo-shaped objects cross stage boundaries; a new bot/capability is a new
  `BotVersion`/`BotPolicy` row + prompt file, not code.
- Capture verification evidence under `openspec/changes/<name>/evidence/`
  (resolver-selected gate output + benchmark-gate results). The gate resolver picks
  per-toolchain gates (Python: ruff/pyright/pytest; Go: go vet / test -race; TS:
  pnpm typecheck/lint/test); the LLM-tier benchmark gates (faithfulness >= 0.85,
  citation accuracy >= 0.95, p95 latency <= 12s, tenant isolation) are the product's
  real contract.
