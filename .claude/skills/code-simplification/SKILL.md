---
name: code-simplification
description: Simplifies code for clarity (MeKnow-adapted, polyglot + OpenSpec). Use when refactoring code for clarity without changing behavior in this repo. Use when code works but is harder to read, maintain, or extend than it should be. Use when reviewing code that has accumulated unnecessary complexity. Every change keeps the resolver-selected gates (uv/go/pnpm) green and respects the repo's invariants.
tags: []
---

# Code Simplification

> Inspired by the [Claude Code Simplifier plugin](https://github.com/anthropics/claude-plugins-official/blob/main/plugins/code-simplifier/agents/code-simplifier.md). Adapted here for the polyglot Mezon Mentor Bot / "MeKnow" project (Python + Go + TS, OpenSpec) as a process-driven skill.

## Overview

Simplify code by reducing complexity while preserving exact behavior. The goal is not fewer lines — it's code that is easier to read, understand, modify, and debug. Every simplification must pass a simple test: "Would a new team member understand this faster than the original?"

This is a polyglot repo. Every simplification must keep the **resolver-selected gates** green for the touched toolchain (Python: ruff/pyright/pytest; Go: go vet / test -race; TS: pnpm typecheck/lint/test) — the gate resolver (`.claude/workflows/lib/gate-resolver.js`) picks them by package — and respect the repo's hard invariants.

## When to Use

- After a feature is working and tests pass, but the implementation feels heavier than it needs to be
- During code review when readability or complexity issues are flagged
- When you encounter deeply nested logic, long functions, or unclear names
- When refactoring code written under time pressure
- When consolidating related logic scattered across files
- After merging changes that introduced duplication or inconsistency

**When NOT to use:**

- Code is already clean and readable — don't simplify for the sake of it
- You don't understand what the code does yet — comprehend before you simplify
- The code is on a latency-sensitive path (retrieval / synthesize loop measured against the p95 <= 12s gate) and the "simpler" version would be measurably slower
- You're about to rewrite the module entirely — simplifying throwaway code wastes effort

## The Five Principles

### 1. Preserve Behavior Exactly

Don't change what the code does — only how it expresses it. All inputs, outputs, side effects, error behavior, and edge cases must remain identical. In this repo this is non-negotiable around the **tenant_id scoping** (no cross-tenant leakage), the **server-side ACL** in `retrieve_kb`, the **citations-mandatory** filter (`refuse_if: no_citations`), **`temperature == 0`** on synthesize/cite/filter, and the **append-only** version lineage (`BotVersion`/`KBVersion`). If you're not sure a simplification preserves behavior, don't make it.

```
ASK BEFORE EVERY CHANGE:
→ Does this produce the same output for every input?
→ Does this maintain the same error behavior (same raised exceptions / wrapped errors / nils)?
→ Does this preserve the same side effects and ordering (DB writes, cache keys, citations)?
→ Is tenant_id still threaded and ACL still server-side?
→ Do all existing tests still pass without modification (the touched toolchain's gates)?
```

### 2. Follow Project Conventions

Simplification means making code more consistent with the codebase, not imposing external preferences. Before simplifying:

```
1. Read openspec/project.md and the relevant openspec/specs/<capability>/spec.md
   (and the docs/design/NNNN-*.md rationale it links)
2. Study how neighboring code handles similar patterns
3. Match the project's style for:
   - Python: ruff-clean (E/F/I/B/UP/ASYNC/SIM, line 100), pyright-clean, anthropic-sdk-python
     directly (no LangChain), tenant_id threaded, memo-shaped objects across stage boundaries
   - Go: wrapped errors fmt.Errorf("...: %w", err), ctx first param, table-driven tests, -race
   - TS (apps/portal): generated OpenAPI types (never hand-written), eslint-clean
```

Simplification that breaks project consistency is not simplification — it's churn.

### 3. Prefer Clarity Over Cleverness

Explicit code is better than compact code when the compact version requires a mental pause to parse.

```python
# UNCLEAR (Python): dense dict-literal indexing to pick a label
label = {True: "Admin", False: "User"}[user.is_admin]

# CLEAR: a small named function with guard returns
def stage_label(stage: Stage) -> str:
    match stage.status:
        case Status.DONE:    return "Done"
        case Status.REFUSED: return "Refused"
        case Status.RUNNING: return "Running"
        case _:              return "Queued"
```

```go
// UNCLEAR (Go): building a map with inline mutation in one expression-ish loop
counts := map[string]int{}
for _, m := range memos { counts[m.TenantID] = counts[m.TenantID] + 1 }

// CLEAR: a named intermediate and the idiomatic increment
countByTenant := make(map[string]int, len(memos))
for _, m := range memos {
	countByTenant[m.TenantID]++
}
```

```ts
// UNCLEAR (TS): nested ternary chain
const tier = u.admin ? (u.super ? "owner" : "admin") : "member";

// CLEAR: explicit branches
function tierOf(u: User): Tier {
  if (!u.admin) return "member";
  return u.super ? "owner" : "admin";
}
```

### 4. Maintain Balance

Simplification has a failure mode: over-simplification. Watch for these traps:

- **Inlining too aggressively** — removing a helper that gave a concept a name (e.g. `enforce_acl`, `assert_tenant`) makes the call site harder to read
- **Combining unrelated logic** — merging two simple functions into one complex function is not simpler
- **Removing "unnecessary" abstraction** — the `kb.*` MCP tool boundary and the `BotPolicy` interpreter exist on purpose (no LangChain, declarative-not-code); don't collapse them into inline calls
- **Optimizing for line count** — fewer lines is not the goal; easier comprehension is

### 5. Scope to What Changed

Default to simplifying recently modified code. Avoid drive-by refactors of unrelated code unless explicitly asked to broaden scope (see `incremental-implementation`'s scope discipline). Unscoped simplification creates noise in diffs and risks unintended regressions.

## The Simplification Process

### Step 1: Understand Before Touching (Chesterton's Fence)

Before changing or removing anything, understand why it exists. This is Chesterton's Fence: if you see a fence across a road and don't understand why it's there, don't tear it down. First understand the reason, then decide if it still applies.

```
BEFORE SIMPLIFYING, ANSWER:
- What is this code's responsibility?
- What calls it? What does it call?
- What are the edge cases and error paths?
- Are there tests that define the expected behavior?
- Why might it have been written this way? (tenant_id scoping? Server-side ACL?
  Citation refusal? temperature==0? Append-only versions? MCP boundary?)
- Check git blame / the relevant openspec change + docs/design rationale: what was
  the original context?
```

If you can't answer these, you're not ready to simplify. Read more context first. The `openspec/project.md` "Hard invariants" list is a catalog of fences that look removable but aren't.

### Step 2: Identify Simplification Opportunities

Scan for these patterns — each one is a concrete signal, not a vague smell:

**Structural complexity:**

| Pattern | Signal | Simplification |
|---------|--------|----------------|
| Deep nesting (3+ levels) | Hard to follow control flow | Guard clauses / early returns |
| Long functions (50+ lines) | Multiple responsibilities | Split into focused functions with descriptive names |
| Nested conditional expressions | Requires a mental stack to parse | `match`/`switch`, or a lookup map built once |
| Boolean parameter flags | `do_thing(True, False)` | Use keyword args / an options struct, or separate functions |
| Repeated conditionals | Same check in several places | Extract a well-named predicate |

**Naming and readability:**

| Pattern | Signal | Simplification |
|---------|--------|----------------|
| Generic names | `data`, `res`, `tmp`, `v`, `x` | Rename to the content: `scoped_chunks`, `cited_answer` |
| Abbreviated names | `usr`, `cfg`, `evt` | Full words unless universal (`id`, `url`, `db`, `ctx`) |
| Misleading names | `get_answer` that also mutates a version | Rename to reflect actual behavior |
| Comments explaining "what" | `# increment counter` above `count += 1` | Delete it — the code is clear |
| Comments explaining "why" | `# server-side ACL: never trust model-supplied identity` | Keep these — they carry intent the code can't express |

**Redundancy:**

| Pattern | Signal | Simplification |
|---------|--------|----------------|
| Duplicated logic | Same 5+ lines in multiple places | Extract a shared function |
| Dead code | Unreachable branches, unused vars, commented-out blocks | Remove (after confirming truly dead; ruff/go vet/eslint help) |
| Unnecessary wrappers | `def get(id): return svc.find(id)` adding nothing | Inline the wrapper |
| Over-engineered patterns | Factory-for-a-factory, an interface/Protocol with one impl added "for tests" | Use the concrete type + a stub at the boundary |
| Redundant conversions/casts | Casting to an already-known type | Remove |

### Step 3: Apply Changes Incrementally

Make one simplification at a time. Run the narrow package's gates, then the suite, after each change. **Submit refactoring changes separately from feature or bug-fix changes.** A PR that refactors and adds a feature is two PRs — split them.

```
FOR EACH SIMPLIFICATION:
1. Make the change
2. Run the touched toolchain's gates on the package:
   - py:  uv --directory D run ruff check . && uv --directory D run pyright && uv --directory D run python -m pytest -q
   - go:  go vet ./... && go test -race ./...
   - ts:  pnpm typecheck && pnpm lint && pnpm test
   (and bash benchmarks/ci-free-gates.sh if a py/bench path changed)
3. If green → keep going (or commit)
4. If red → revert and reconsider
```

Avoid batching multiple simplifications into a single untested change. If something breaks, you need to know which simplification caused it.

**The Rule of 500:** If a refactoring would touch more than 500 lines, invest in automation (codemods, `ruff`/`gofmt -r` rewrite rules, `ts-morph`, or `sed`) rather than editing by hand. Manual edits at that scale are error-prone and exhausting to review.

### Step 4: Verify the Result

After all simplifications, step back and evaluate the whole:

```
COMPARE BEFORE AND AFTER:
- Is the simplified version genuinely easier to understand?
- Did you introduce any pattern inconsistent with the codebase?
- Is the diff clean and reviewable?
- Would a teammate approve this change?
```

If the "simplified" version is harder to understand or review, revert. Not every simplification attempt succeeds.

## Language-Specific Guidance

### Python

```python
# SIMPLIFY: pointless wrapper that just forwards
# Before
def get_user(ctx, id): return user_service.find_by_id(ctx, id)
# After — call user_service.find_by_id directly; delete the wrapper.

# SIMPLIFY: deep nesting → guard clauses
# Before
def synthesize(req):
    if req is not None:
        if req.has_citations():
            if req.tenant_id:
                return answer(req)
            raise ValueError("missing tenant_id")
        raise NoCitations()      # refuse rather than emit ungrounded claims
    raise ValueError("req is nil")
# After
def synthesize(req):
    if req is None:
        raise ValueError("req is nil")
    if not req.tenant_id:
        raise ValueError("missing tenant_id")
    if not req.has_citations():
        raise NoCitations()      # refuse rather than emit ungrounded claims
    return answer(req)

# SIMPLIFY: redundant boolean
# Before
def is_valid(s): 
    if 0 < len(s) < 100: return True
    return False
# After
def is_valid(s): return 0 < len(s) < 100
```

### Go

```go
// SIMPLIFY: verbose conditional assignment
// Before
var name string
if u.Nickname != "" { name = u.Nickname } else { name = u.FullName }
// After
name := u.Nickname
if name == "" { name = u.FullName }

// SIMPLIFY: manual slice building (Go has no stdlib filter — keep the loop, name the predicate)
active := make([]User, 0, len(users))
for _, u := range users {
	if isActive(u) {
		active = append(active, u)
	}
}
```

### TypeScript

```ts
// SIMPLIFY: redundant boolean wrapper
// Before
function ok(s: string): boolean { return s.length > 0 ? true : false; }
// After
function ok(s: string): boolean { return s.length > 0; }
```

**Do not "simplify" away these on-purpose patterns:**

- **`tenant_id` threading** through queries, cache keys, and logs — it looks like noise but it is the multi-tenant isolation fence (cross-tenant joins are bugs).
- **Server-side ACL** in `retrieve_kb` — never refactor it to trust model-supplied identity.
- The **citations-mandatory** refusal (`refuse_if: no_citations`) and **`temperature == 0`** on synthesize/cite/filter — these are answer-trust fences.
- The **`kb.*` MCP tool boundary** and the declarative **`BotPolicy` interpreter** — even with one implementation, they keep tools/policies data-not-code (and there is no LangChain).
- **Append-only version lineage** — never collapse a "new child `BotVersion`/`KBVersion`" into an in-place mutation.
- The **compression invariant** — only `memo_schema_ref`-shaped objects cross stage boundaries; don't pass raw KB chunks out of a sub-agent.
- **Wrapped errors** (Go `%w`) collapsed to bare strings — that loses `errors.Is/As` matching elsewhere.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It's working, no need to touch it" | Working code that's hard to read is hard to fix when it breaks. Simplifying now saves time on every future change. |
| "Fewer lines is always simpler" | A one-line nested expression is not simpler than a 5-line `match`/`switch`. Simplicity is comprehension speed, not line count. |
| "I'll just quickly simplify this unrelated code too" | Unscoped simplification creates noisy diffs and risks regressions. Stay focused (see `incremental-implementation`). |
| "The types make it self-documenting" | Types document structure, not intent. A well-named function explains *why* better than a signature explains *what*. |
| "This abstraction might be useful later" | Don't preserve speculative abstractions. If it's unused now, remove it and re-add when needed — unless it's a documented invariant fence (MCP boundary, BotPolicy interpreter). |
| "The original author must have had a reason" | Maybe. Check git blame and the openspec change + docs/design rationale — apply Chesterton's Fence. The `openspec/project.md` invariants list is the catalog of real reasons. |
| "I'll refactor while adding this feature" | Separate refactoring from feature work. Mixed changes are harder to review, revert, and read in history. |

## Red Flags

- Simplification that requires modifying tests to pass (you likely changed behavior)
- "Simplified" code that is longer and harder to follow than the original
- Renaming things to match preferences rather than project conventions
- Removing error handling/wrapping or an exception path because "it's cleaner"
- Dropping `tenant_id` from a query/cache key, or moving ACL off the server
- Weakening the citation refusal or raising `temperature` above 0
- Collapsing the `kb.*` MCP boundary or the `BotPolicy` interpreter; introducing LangChain
- Mutating a version in place instead of appending a child
- Simplifying code you don't fully understand
- Batching many simplifications into one large, hard-to-review commit
- Refactoring code outside the current task scope without being asked

## Verification

After completing a simplification pass:

- [ ] All existing tests pass without modification (the touched toolchain's resolver-selected gates)
- [ ] Touched packages build/import: `uv --directory D run python -c 'import <pkg>'` / `go build ./...` / `pnpm build` as applicable
- [ ] Lint/format/type gates pass: py `ruff check .` + `ruff format --check .` + `pyright`; go `go vet ./...`; ts `pnpm lint` + `pnpm typecheck`
- [ ] Benchmark gates green if a py/bench path changed: `bash benchmarks/ci-free-gates.sh`
- [ ] Each simplification is a reviewable, incremental change
- [ ] The diff is clean — no unrelated changes mixed in
- [ ] Simplified code follows project conventions (checked against `openspec/project.md` and the relevant `openspec/specs/`)
- [ ] No error handling or error wrapping was removed or weakened
- [ ] No invariant fence was torn down (tenant_id everywhere, server-side ACL, citations-mandatory, temperature==0, append-only versions, MCP boundary, no LangChain)
- [ ] No dead code left behind (unused imports/vars — ruff/go vet/eslint confirm)
- [ ] A teammate or review agent would approve the change as a net improvement

**Note:** A unit lives in exactly one toolchain (py/go/ts); run that toolchain's gates. If a simplification touches a migration or DB-backed path, bring up the dependencies and run the migration/coverage gate — a green unit test on a stubbed store is not proof you preserved DB behavior. `openspec validate "<change>" --strict` always applies.

## MeKnow notes

- **OpenSpec lifecycle.** A pure simplification rarely needs a new spec, but if it
  changes observable behavior it does — route it through `/opsx:propose` →
  `/opsx:spec` → `/opsx:spec-pr` → `/opsx:ship` → `/opsx:archive`, and never let a
  "simplification" silently alter a baselined spec under `openspec/specs/`. Keep
  refactor commits out of feature/`/opsx:ship` change sets; ship them on their own.
  All OpenSpec work happens inside this `platform/` submodule.
- **Invariant fences (do not simplify away):** `tenant_id` on every table/query/cache
  key/log line (no cross-tenant joins); ACL enforced **server-side** in `retrieve_kb`
  (never trust model-supplied identity); citations mandatory (`refuse_if:
  no_citations`); `temperature == 0` on synthesize/cite/filter; versions **append-only**
  (`BotVersion`/`KBVersion` — new child, never mutate); only `memo_schema_ref`-shaped
  objects cross stage boundaries; MCP is the internal tool boundary and there is **no
  LangChain**; a new bot/capability is a new `BotPolicy`/`BotVersion` row + prompt file,
  not code (and a new engine capability is a registry row, not an interpreter branch).
- **Verify with the repo's tooling:** the gate resolver picks per-toolchain gates
  (Python: ruff/pyright/pytest; Go: go vet / test -race; TS: pnpm typecheck/lint/test),
  plus `bash benchmarks/ci-free-gates.sh` and `openspec validate "<change>" --strict`.
  The LLM-tier benchmark gates (faithfulness >= 0.85, citation accuracy >= 0.95, p95
  latency <= 12s, tenant isolation) are the product's real contract. Pair with
  `test-driven-development` (a refactor that needs test edits changed behavior) and
  `incremental-implementation` (one simplification per tested step).
