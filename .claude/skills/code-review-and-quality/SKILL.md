---
name: code-review-and-quality
description: Conducts multi-axis code review (MeKnow-adapted, polyglot). Use before merging any change. Use when reviewing code written by yourself, another agent, or a human across Python/Go/TypeScript. Use when you need to assess code quality across correctness, readability, architecture, security, and performance before it enters main. Complements the built-in `/code-review` and `/security-review` slash commands and the `/opsx:address-review` PR-comment loop.
tags: []
---

# Code Review and Quality

## Overview

Multi-dimensional code review with quality gates. Every change gets reviewed before merge — no exceptions. Review covers five axes: correctness, readability, architecture, security, and performance.

This skill is the methodology behind, and complements, the repo's automation:
- the built-in **`/code-review`** slash command (diff-focused correctness + cleanup pass),
- the built-in **`/security-review`** slash command (security-focused pass — see the `security-and-hardening` sibling skill for the controls),
- the **`/opsx:address-review`** loop, which applies PR-comment feedback against an OpenSpec change.

Use those for the mechanics; use this skill for the standard you hold the change to.

**The approval standard:** Approve a change when it definitely improves overall code health, even if it isn't perfect. Perfect code doesn't exist — the goal is continuous improvement. Don't block a change because it isn't exactly how you would have written it. If it improves the codebase, follows the conventions of the touched language (idiomatic Python / Go / TypeScript), and upholds the hard invariants in `openspec/project.md`, approve it.

## When to Use

- Before merging any PR or change (including before `/opsx:ship` opens a PR)
- After completing a feature implementation via `/opsx:ship-code`
- When another agent or model produced code you need to evaluate
- When refactoring existing packages
- After any bug fix (review both the fix and the regression test — see `debugging-and-error-recovery`)

## The Five-Axis Review

Every review evaluates code across these dimensions:

### 1. Correctness

Does the code do what it claims to do?

- Does it match the OpenSpec delta / task requirements?
- Are edge cases handled (None/nil, empty list/dict, zero values, boundary values)?
- Are error paths handled? Exceptions caught at the right boundary (Python); every returned `error` checked and wrapped with `%w` where inspected (Go); rejected promises handled (TS). Errors are not swallowed.
- Does it pass the resolver-selected gates? Did the DB-dependent pytest actually run against pgvector (`DATABASE_URL`/`TEST_DATABASE_URL` set), or silently skip?
- Are there off-by-one errors, **race conditions**, or state inconsistencies? For Go concurrency, check with `go test -race`. For async Python, check the `ASYNC`-lint findings and that awaited work isn't dropped.
- Does it preserve the hard invariants — `temperature == 0` on synthesize/cite/filter, `refuse_if: no_citations` in the filter stage, ACL enforced server-side in `retrieve_kb`?

### 2. Readability & Simplicity

Can another engineer (or agent) understand this code without the author explaining it?

- Are names idiomatic for the language and consistent with the package? (No `temp`, `data`, `result` without context; clear module/function names; no stutter.)
- Is the control flow straightforward? Prefer early returns and guard clauses over deep nesting.
- Is the code organized logically (related code grouped, clear package boundaries — `packages/*` vs `apps/*` vs `apps/portal`)?
- Are there "clever" tricks (reflection, metaprogramming, unsafe, channel gymnastics) that should be simplified?
- **Could this be done in fewer lines?** (1000 lines where 100 suffice is a failure.)
- **Are abstractions earning their complexity?** Don't add a generic helper or a new lobe/path until a second/third caller needs it. The BotPolicy interpreter is the one deliberate generalization — a new bot behavior is a config row + prompt file, not a new interpreter branch; match that level, don't out-abstract it.
- Would a doc comment clarify non-obvious intent? (But don't comment obvious code.)
- Any dead code: unused exports, no-op assignments, backwards-compat shims, `# removed` / `// removed` comments?

### 3. Architecture

Does the change fit the system's design?

- Does it follow existing patterns or introduce a new one? If new, is it justified?
- Does it respect the package boundaries (`packages/{rag-core,agent-core,ingest-core,llm-transport,...}`, `apps/*`, `apps/portal`)? Does the dependency flow point the right way?
- Is there code duplication that should be shared?
- Is a new bot/capability expressed as a **new `BotVersion`/`BotPolicy` row + prompt file** (not code), and a new engine capability as a **lobe/path registry row** (not an interpreter branch)? If a "new bot" needs an interpreter change, the abstraction is wrong.
- Does it honor the framework boundaries: **no LangChain/LangGraph/Haystack/LlamaIndex** (`anthropic-sdk-python` directly), **MCP as the internal tool boundary** (`kb.*` contract, embedded transport, ACP/A2A out of the retrieval loop), the **compression invariant** (only `memo_schema_ref`-shaped objects cross stage boundaries)?
- Is **OpenAPI the API contract** — backend generates the spec, portal types are *generated*, never hand-written?
- Does the change belong to an OpenSpec change? Non-trivial behavior changes should have a spec delta, not just code.

### 4. Security

For detailed security guidance, see the `security-and-hardening` sibling skill and the built-in `/security-review` command. In review specifically, check:

- Is external input (chat messages, ingested KB documents, connector payloads, portal requests) validated at the boundary before use, and treated as **untrusted data, not instructions**?
- Is **ACL enforced server-side inside `retrieve_kb`**, with caller identity inherited from the request and **never trusted from the model**?
- Is the system **multi-tenant by default** — does every query, cache key, and log line carry `tenant_id`, with no cross-tenant joins and a cache key that includes the ACL-cohort hash?
- Are SQL queries parameterized (`$1`/`%s`/bound params), never string-concatenated?
- Are secrets kept out of code, logs, and version control? No API keys, tokens, or unsealed credentials in log lines or error strings.
- Are **versions append-only** — is the change adding a new child with a parent pointer rather than mutating a prior `BotVersion`/`KBVersion`/golden set/trace in place?
- Does the **filter stage refuse rather than emit ungrounded claims** (`refuse_if: no_citations`), and is `temperature == 0` held on synthesize/cite/filter so output is deterministic and not coaxable?
- Does the **compression invariant** hold — raw KB chunks never leave a sub-agent across a stage boundary?

### 5. Performance

Does the change introduce performance problems?

- Any N+1 query patterns against pgvector/Postgres? Prefer a single query / `IN` over a loop of queries. Is the vector search bounded (top-k) and the ACL filter pushed into the query?
- Any unbounded loops or unconstrained data fetching (e.g. ingesting a whole KB into memory)?
- Any blocking call inside an async path or request handler that should have a timeout/context? Latency matters — the product gate is **p95 <= 12s**.
- Any task/worker leaks — an asyncio task or goroutine started without a way to stop it, a channel never drained?
- Any large allocation or redundant embedding computation in a hot path?
- Any missing pagination on list endpoints?

## Change Sizing

Small, focused changes are easier to review, faster to merge, and safer to deploy. Target these sizes:

```
~100 lines changed   → Good. Reviewable in one sitting.
~300 lines changed   → Acceptable if it's a single logical change.
~1000 lines changed  → Too large. Split it.
```

A single OpenSpec change should ideally map to a reviewable diff. If a change's tasks add up to a 1000-line diff, the change itself was probably too big — split the proposal.

**What counts as "one change":** A single self-contained modification that addresses one thing, includes related tests in the touched package's language, and keeps the resolver-selected gates green after submission. One part of a feature — not the whole feature.

**Splitting strategies when a change is too large:**

| Strategy | How | When |
|----------|-----|------|
| **Stack** | Submit a small change, start the next based on it | Sequential dependencies |
| **By package** | Separate changes for `packages/rag-core` vs `apps/backend` vs `apps/portal` etc. | Cross-cutting concerns |
| **Horizontal** | Add the shared contract/migration first, then consumers | Layered work (e.g. new `kb.*` MCP tool, OpenAPI schema) |
| **Vertical** | Break into smaller full-pipeline slices | Feature work (ingest → retrieve_kb → synthesize/cite/filter → API) |

**When large changes are acceptable:** Complete file deletions and automated refactoring (e.g. `ruff format` / `gofmt` / rename) where the reviewer only needs to verify intent, not every line. A new Alembic migration plus its generated touchpoints, or regenerated OpenAPI/portal types, can be large but is still one logical change.

**Separate refactoring from feature work.** A change that refactors an existing package and adds new behavior is two changes — submit them separately. Small cleanups (a rename, a format pass) can be included at reviewer discretion.

## Change Descriptions

Every change needs a description that stands alone in version-control history.

**First line:** Short, imperative, standalone. "Enforce tenant ACL server-side in retrieve_kb" not "ACL stuff." Informative enough that someone searching history understands it without the diff.

**Body:** What is changing and why. Include context, decisions, and reasoning not visible in the code. Link to the OpenSpec change name, the design doc (`docs/design/NNNN-<slug>.md`), benchmark results, or the relevant capability spec. Acknowledge approach shortcomings when they exist.

**Anti-patterns:** "Fix bug," "Fix build," "Add patch," "Moving code from A to B," "Phase 1," "Add convenience functions."

End commit messages with the required co-author trailer
(`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`).

## Review Process

### Step 1: Understand the Context

Before looking at code, understand the intent:

```
- What is this change trying to accomplish?
- Which OpenSpec change / spec delta does it implement?
- What is the expected behavior change?
```

### Step 2: Review the Tests First

Tests reveal intent and coverage:

```
- Do tests exist, in the touched package's language (tests/test_<feature>.py,
  <feature>_test.go, apps/portal/src/**/<feature>.test.ts(x))?
- Do they test behavior (not implementation details)?
- Are edge cases covered (None/nil, empty, error paths, boundary)?
- Do tests have descriptive names (test_filter_refuses_when_no_citations)?
- Do they seed at least two tenants and assert results never cross tenant_id?
- Are LLM calls stubbed deterministically (temperature == 0), not hitting a live model?
- Do behavioral guarantees reference the right benchmarks/gates/ script?
- Do DB-dependent pytest run against pgvector, and skip cleanly without a DB?
```

### Step 3: Review the Implementation

Walk through the code with the five axes in mind:

```
For each file changed:
1. Correctness: Does this code do what the test says it should? Errors handled? Invariants held?
2. Readability: Idiomatic for its language? Can I understand it without help?
3. Architecture: Does it fit the package boundaries, the BotPolicy/lobe model, MCP boundary, no-LangChain rule?
4. Security: untrusted-input handling, server-side ACL, tenant_id everywhere, parameterized queries, no secrets in logs, append-only versions, refuse_if no_citations?
5. Performance: N+1, unbounded fetch/embedding, task/goroutine leaks, missing timeouts, p95 budget?
```

### Step 4: Categorize Findings

Label every comment with its severity so the author knows what's required vs optional:

| Prefix | Meaning | Author Action |
|--------|---------|---------------|
| *(no prefix)* | Required change | Must address before merge |
| **Critical:** | Blocks merge | Cross-tenant leak, ACL trusted from the model, ungrounded answer (no `refuse_if`), non-zero temperature on synthesize/cite/filter, secret in logs, in-place mutation of an append-only version |
| **Nit:** | Minor, optional | Author may ignore — format-clean style, naming preference |
| **Optional:** / **Consider:** | Suggestion | Worth considering, not required |
| **FYI** | Informational only | No action needed — context for the future |

This prevents authors from treating all feedback as mandatory.

### Step 5: Verify the Verification

Check the author's verification story:

```
- Did the resolver-selected gates pass for the touched packages
  (uv ruff/format/pyright/pytest, go build/vet/test -race, pnpm typecheck/lint/test)?
- Did bash benchmarks/ci-free-gates.sh run for py/bench changes?
- Did openspec validate "<change>" --strict pass?
- Did the DB-dependent pytest actually run against pgvector, or skip?
- For behavioral guarantees, did the LLM-tier judge gates meet the thresholds
  (faithfulness >= 0.85, citation accuracy >= 0.95, p95 latency <= 12s,
  cross-tenant isolation pass, ACL escape pass)?
```

## Multi-Model / Multi-Agent Review Pattern

Use different agents for different review perspectives:

```
Agent A writes the code (e.g. via /opsx:ship-code)
    │
    ▼
Agent B reviews for correctness and architecture (this skill / /code-review)
    │
    ▼
Agent A addresses the feedback (/opsx:address-review)
    │
    ▼
Human makes the final call
```

Different models have different blind spots. AI-generated code often runs and looks plausible while ignoring an unhandled error, a dropped await, a leaked task, or a broken invariant — review it harder, not softer.

**Example prompt for a review agent:**
```
Review this change for correctness, security, and our project.md invariants.
The OpenSpec change is [X]. The change should [Y]. Confirm ACL is enforced
server-side in retrieve_kb (never trusted from the model), every query/cache
key/log line carries tenant_id, temperature == 0 on synthesize/cite/filter,
the filter stage has refuse_if no_citations, versions stay append-only, no
LangChain creeps in, and raw KB chunks don't cross a stage boundary. Flag
issues as Critical / Important / Suggestion.
```

## Dead Code Hygiene

After any refactoring or implementation change, check for orphaned code:

1. Identify code now unreachable or unused (`ruff` F401/F811 + `pyright` for Python; `go vet` / `staticcheck` for Go; `eslint` / `tsc` for TS; or grep for the symbol).
2. List it explicitly.
3. **Ask before deleting:** "Should I remove these now-unused elements: [list]?"

Don't leave dead code lying around — it confuses future readers and agents. But don't silently delete things you're unsure about.

```
DEAD CODE IDENTIFIED:
- format_legacy_answer() in packages/rag-core/synth.py — replaced by format_answer()
- the pre-MCP direct-KB call path in packages/agent-core — superseded by the kb.* tool
- unused LEGACY_EMBED_MODEL constant in packages/ingest-core — no remaining references
→ Safe to remove these?
```

## Review Speed

Slow reviews block the whole flow. The cost of context-switching to review is less than the waiting cost imposed on others.

- **Respond within one business day** — maximum, not target.
- **Ideal cadence:** respond shortly after a review request arrives, unless deep in focused coding.
- **Prioritize fast individual responses** over quick final approval. Quick feedback reduces frustration even across multiple rounds.
- **Large changes:** ask the author to split them rather than reviewing one massive changeset.

## Handling Disagreements

When resolving review disputes, apply this hierarchy:

1. **Technical facts and data** override opinions and preferences.
2. **Language style** (`ruff`/`ruff format` + PEP 8 for Python, `gofmt`/Effective Go for Go, `eslint`/`prettier` for TS) is the authority on style matters.
3. **The `openspec/project.md` hard invariants** are non-negotiable — a change that breaks one doesn't merge regardless of preference.
4. **Codebase consistency** is acceptable if it doesn't degrade overall health.

**Don't accept "I'll clean it up later."** Deferred cleanup rarely happens. Require cleanup before submission unless it's a genuine emergency; otherwise file a tracking ticket with self-assignment.

## Honesty in Review

When reviewing code — yours, another agent's, or a human's:

- **Don't rubber-stamp.** "LGTM" without evidence of review helps no one.
- **Don't soften real issues.** Calling a cross-tenant leak "a minor concern" is dishonest.
- **Quantify problems when possible.** "This adds one DB round-trip per retrieved chunk, pushing p95 past the 12s gate" beats "this could be slow."
- **Push back on approaches with clear problems.** Sycophancy is a failure mode in reviews. Say it directly and propose an alternative.
- **Accept override gracefully.** If the author has full context and disagrees, defer. Comment on code, not people.

## Dependency Discipline

Part of code review is dependency review.

**Before adding any dependency (PyPI / Go module / npm):**
1. Does the existing stack solve this? (`anthropic-sdk-python`, `sentence-transformers`, the standard library, pgvector, the existing `packages/*` — often it does.)
2. Is it actively maintained? (Last commit, open issues.)
3. Does it have known vulnerabilities? (`pip-audit` / `govulncheck ./...` / `pnpm audit`.)
4. What's the license? (Must be compatible — this repo vendors MIT skills, for example.)
5. Will the toolchain stay clean (`uv` lock, `go mod tidy`, `pnpm install --frozen-lockfile`)?

**Rule:** Prefer the standard library and existing `packages/*` utilities over new dependencies. Every dependency is a liability and attack surface. **Hard stop:** no LangChain/LangGraph/Haystack/LlamaIndex or other agent framework — the runtime is built on `anthropic-sdk-python` directly.

## The Review Checklist

```markdown
## Review: [PR/Change title]

### Context
- [ ] I understand what this change does and which OpenSpec change it implements

### Correctness
- [ ] Change matches the spec delta / task requirements
- [ ] Edge cases handled (None/nil, empty, zero values, boundaries)
- [ ] Errors handled at the right boundary; not swallowed
- [ ] Invariants preserved (temperature == 0; refuse_if no_citations; server-side ACL)
- [ ] Tests cover the change; -race run for Go concurrency

### Readability
- [ ] Names idiomatic and consistent; format-clean (ruff/gofmt/prettier)
- [ ] Early-return control flow, not deep nesting
- [ ] No unnecessary complexity or premature abstraction

### Architecture
- [ ] Respects package boundaries (packages/* vs apps/* vs apps/portal) and dependency direction
- [ ] New bot/capability = BotVersion/BotPolicy row + prompt file (not code); new engine cap = lobe registry row
- [ ] MCP boundary honored (kb.* contract, embedded transport); no LangChain; compression invariant holds
- [ ] OpenAPI is the contract (portal types generated, not hand-written)

### Security
- [ ] External input (chat/KB docs/connectors/portal) validated and treated as untrusted
- [ ] ACL enforced server-side in retrieve_kb; caller identity never trusted from the model
- [ ] tenant_id on every query, cache key, and log line; no cross-tenant joins
- [ ] SQL parameterized; no string concatenation
- [ ] No secrets/keys/tokens in logs or version control
- [ ] Versions append-only (new child + parent pointer, no in-place mutation)

### Performance
- [ ] No N+1 against pgvector; vector search bounded (top-k), ACL pushed into the query
- [ ] No unbounded fetch/embedding; no task/goroutine leaks
- [ ] Blocking calls have timeout/context; within the p95 <= 12s budget
- [ ] Pagination on list endpoints

### Verification
- [ ] Resolver-selected gates pass (uv/go/pnpm as applicable + ci-free-gates.sh + openspec validate --strict)
- [ ] DB-dependent pytest actually ran against pgvector (DATABASE_URL set), not skipped
- [ ] Behavioral guarantees meet thresholds (faithfulness >= 0.85, citation >= 0.95, p95 <= 12s, tenant isolation, ACL escape)

### Verdict
- [ ] **Approve** — Ready to merge
- [ ] **Request changes** — Issues must be addressed
```

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It works, that's good enough" | Working code that's unreadable, insecure, or breaks an invariant creates debt that compounds. |
| "I wrote it, so I know it's correct" | Authors are blind to their own assumptions. Every change benefits from another set of eyes. |
| "We'll clean it up later" | Later never comes. The review is the quality gate — require cleanup before merge. |
| "AI-generated code is probably fine" | AI code needs more scrutiny, not less. It's confident and plausible, even when it drops an await, leaks a tenant, or skips `refuse_if`. |
| "The tests pass, so it's good" | Tests are necessary, not sufficient — and the DB-dependent ones may have skipped without a pgvector instance. They don't catch architecture or readability problems. |

## Red Flags

- PRs merged without any review
- Review that only checks if tests pass (ignoring the other four axes)
- "LGTM" without evidence of actual review
- Security-sensitive changes (ACL, tenancy, secrets, prompt/filter handling) without a `/security-review` pass
- Large PRs that are "too big to review properly" (split them / split the OpenSpec change)
- No regression test with a bug-fix PR
- A green CI that skipped DB-dependent pytest (no pgvector)
- A behavioral change that doesn't move the relevant benchmark gate
- Review comments without severity labels
- Accepting "I'll fix it later"

## MeKnow notes

- This skill complements the built-in **`/code-review`** and **`/security-review`** slash commands and the **`/opsx:address-review`** PR-comment loop. Run those for the mechanics; hold the change to this five-axis standard.
- **Lifecycle:** non-trivial work flows through OpenSpec — `/opsx:propose` → `/opsx:spec` → `/opsx:spec-pr` → `/opsx:ship` (ship-plan → ship-code, with review this skill backs + reconcile) → `/opsx:address-review` → `/opsx:archive`, all inside the `platform/` submodule. The autonomous `/opsx:ship` pipeline gates on the resolver-selected gates, so a review that lets a lint-failing, type-failing, or test-skipping change through will stall it.
- **Invariants to verify every time** (from `openspec/project.md`): multi-tenant by default (`tenant_id` on every table/query/cache key/log line; cross-tenant joins are bugs; cache keys include an ACL-cohort hash); citations mandatory (`filter` stage carries `refuse_if: no_citations`); `temperature == 0` on `synthesize`/`cite`/`filter`; ACL enforced server-side in `retrieve_kb` with caller identity inherited (never trusted from the model); versions append-only (`BotVersion`/`KBVersion`/golden sets/traces — new child with a parent pointer); the compression invariant (only `memo_schema_ref`-shaped objects cross stage boundaries); MCP as the internal tool boundary (`kb.*`, embedded transport, ACP/A2A out of the retrieval loop); no LangChain/LangGraph/Haystack/LlamaIndex; OpenAPI is the API contract (portal types generated); new bot/capability = `BotVersion`/`BotPolicy` row + prompt file; new engine capability = a lobe/path registry row.
- **Build/test gates (polyglot, resolver-selected):**
  - Python (uv member dir `D`): `uv --directory D run ruff check .`, `ruff format --check .`, `pyright`, `python -m pytest -q` (coverage `pytest -q --cov --cov-report=term-missing`).
  - Go (module dir `M`, pinned go 1.24): `go build ./...`, `go vet ./...`, `go test -race ./...` (coverage `go test -race -coverprofile=cover.out ./... && go tool cover -func=cover.out | tail -1`).
  - TypeScript (`apps/portal`): `pnpm typecheck`, `pnpm lint`, `pnpm test`, `pnpm test:e2e` (gated tier).
  - Bench: `bash benchmarks/ci-free-gates.sh` (deterministic ~2min ladder, on every py/bench change).
  - Always: `openspec validate "<change>" --strict`.
  - **Benchmark thresholds (the product's real contract):** faithfulness >= 0.85, citation accuracy >= 0.95, p95 latency <= 12s, plus cross-tenant isolation and ACL-escape gates — see `benchmarks/gates/` and `tests/fixtures/golden_set.json`.
- **DB-dependent pytest** needs `DATABASE_URL`/`TEST_DATABASE_URL` pointing at a pgvector instance (local `docker compose -f docker-compose.dev.yml`; CI `pgvector/pgvector:pg16`). Absent a DB those tests skip (recorded, not failed). Use the `security-and-hardening` and `debugging-and-error-recovery` sibling skills for the security and root-cause depth this review references.

## Verification

After review is complete:

- [ ] All Critical issues are resolved
- [ ] All Important issues are resolved or explicitly deferred with justification
- [ ] The resolver-selected gates pass (DB-dependent pytest actually ran against pgvector)
- [ ] The `openspec/project.md` invariants are intact
- [ ] Behavioral guarantees meet the benchmark thresholds where the change affects them
- [ ] The verification story is documented (what changed, how it was verified)
