---
name: incremental-implementation
description: Delivers changes incrementally (MeKnow-adapted, polyglot + OpenSpec). Use when implementing any feature or change that touches more than one file in this repo. Use when you're about to write a large amount of code at once, or when a task feels too big to land in one step. Each slice stays buildable and green under the resolver-selected gates (uv/go/pnpm as applicable + ci-free-gates.sh + openspec validate).
tags: []
---

# Incremental Implementation

## Overview

Build in thin vertical slices — implement one piece, test it, verify it, then expand. Avoid implementing an entire feature in one pass. Each increment should leave the system in a working, testable state. This is the execution discipline that makes large features manageable, and it is exactly how `/opsx:ship-code` works through an OpenSpec change's work-units: one unit, Red→Green→one commit (ticking `tasks.md` off), before the next.

## When to Use

- Implementing any multi-file change
- Working through the work-units of an OpenSpec change (`/opsx:ship-code`)
- Building a new feature from a task breakdown
- Refactoring existing code
- Any time you're tempted to write more than ~100 lines before testing

**When NOT to use:** Single-file, single-function changes where the scope is already minimal.

## The Increment Cycle

```
┌──────────────────────────────────────┐
│                                      │
│   Implement ──→ Test ──→ Verify ──┐  │
│       ▲                           │  │
│       └───── Commit ◄─────────────┘  │
│              │                       │
│              ▼                       │
│          Next slice                  │
│                                      │
└──────────────────────────────────────┘
```

For each slice:

1. **Implement** the smallest complete piece of functionality
2. **Test** — run the narrow test target in the touched package's language; write the test first per `test-driven-development`
3. **Verify** — the resolver-selected gates stay green (uv/go/pnpm as applicable for the touched packages + `ci-free-gates.sh` on py/bench changes + `openspec validate "<change>" --strict`)
4. **Commit** — save progress with a descriptive message; tick off the matching `tasks.md` item if you're inside an OpenSpec change
5. **Move to the next slice** — carry forward, don't restart

## Slicing Strategies

### Vertical Slices (Preferred)

Build one complete path through the stack. In MeKnow, a "stack" is ingest → retrieve_kb → synthesize/cite/filter → API/portal:

```
Slice 1: filter stage refuses on missing citations   (rag-core filter.py + test_filter.py)
    → unit test passes, refuse_if no_citations enforced

Slice 2: retrieve_kb enforces tenant ACL server-side  (rag-core retrieve + pgvector test)
    → DB test passes, no chunk crosses tenant_id

Slice 3: synthesize emits memo_schema_ref-shaped output (agent-core, temperature==0)
    → deterministic test passes, raw chunks never leave the sub-agent

Slice 4: API surface exposes the answer endpoint        (apps/backend + generated portal types)
    → endpoint test passes, benchmark ladder green
```

Each slice delivers working end-to-end functionality.

### Contract-First Slicing

When the backend and portal (or two packages) evolve in parallel, pin the contract first. OpenAPI is the API contract and portal types are *generated*, never hand-written:

```
Slice 0: Define the contract (OpenAPI schema in apps/backend; regenerate portal types)
Slice 1a: Implement the backend side against the spec + pytest tests
Slice 1b: Implement the portal side against the generated types + vitest tests
Slice 2: Integrate and exercise the real path (benchmark ladder / pnpm test:e2e)
```

For the MCP boundary, pin the `kb.*` tool contract (capability `mcp-and-skills`) first, then implement the embedded-transport tool and its caller against it.

### Risk-First Slicing

Tackle the riskiest or most uncertain piece first:

```
Slice 1: Prove cross-tenant isolation holds in retrieve_kb under a cache-key collision (highest risk)
Slice 2: Build the citation/cite stage on the proven retrieval path
Slice 3: Add the synthesize + filter (refuse_if no_citations) stages on top
```

If Slice 1 fails, you discover it before investing in the later slices.

## Implementation Rules

### Rule 0: Simplicity First

Before writing any code, ask: "What is the simplest thing that could work?" After writing it, review against these checks — and if the answer is "simplify," reach for the `code-simplification` skill:

- Can this be done in fewer lines?
- Are these abstractions earning their complexity?
- Would a staff engineer say "why didn't you just..."?
- Am I building for hypothetical future requirements, or the current task?

```
SIMPLICITY CHECK:
✗ A generic agent-framework layer wrapping the LLM
✓ The thin BotPolicy interpreter on anthropic-sdk-python directly (no LangChain)

✗ A new interpreter branch for a new bot behavior
✓ A new BotVersion/BotPolicy row + prompt file (config, not code)

✗ An interface with a single implementation, added "for testing"
✓ The concrete type plus a stubbed LLM transport at the boundary
```

Three similar lines is better than a premature abstraction. Implement the naive, obviously-correct version first. Optimize only after correctness is proven with tests.

### Rule 0.5: Scope Discipline

Touch only what the task requires.

Do NOT:
- "Clean up" code adjacent to your change
- Reorder imports in files you're not modifying
- Remove comments you don't fully understand
- Add features not in the spec because they "seem useful"
- Modernize syntax in files you're only reading

If you notice something worth improving outside scope, note it — don't fix it:

```
NOTICED BUT NOT TOUCHING:
- packages/ingest-core has an unused helper (unrelated to this task)
- The retrieve_kb log lines could be clearer (separate task)
→ Want me to open a follow-up OpenSpec change for these?
```

### Rule 1: One Thing at a Time

Each increment changes one logical thing. Don't mix concerns.

**Bad:** one commit that adds a new BotPolicy stage, refactors the retrieve_kb ACL path, and edits an Alembic migration.

**Good:** three separate commits — one per change.

### Rule 2: Keep It Buildable

After each increment, the touched packages must still build and their existing tests pass — `uv --directory D run pyright` + `pytest -q` for Python, `go build ./...` for a Go module, `pnpm typecheck` for the portal. Don't leave a package in a broken state between slices. Remember a new bot/capability is a **new `BotVersion`/`BotPolicy` row + prompt file**, not a code change, and a new engine capability is a **lobe/path registry row**, never an interpreter branch — so those slices shouldn't drag code changes along with them.

### Rule 3: Guard Incomplete Behavior

If a capability isn't ready but you want to land increments, keep it inert behind config rather than half-wired into the live path. Example: a new `BotPolicy` row can exist without being the active `BotVersion`, or a behavior can be gated on a policy/config flag that defaults off, so the live retrieval and answer paths stay unchanged until it's complete.

### Rule 4: Safe Defaults

New code defaults to safe, conservative behavior. Honor the invariants:

```python
# Safe: tenant_id is mandatory and threaded through; citations-mandatory means
# the default outcome of an answer path is to refuse, not to emit ungrounded text.
def answer(*, query: str, tenant_id: str, acl: Acl) -> Answer:
    hits = retrieve_kb(query=query, tenant_id=tenant_id, acl=acl)  # ACL server-side
    memo = synthesize(hits, tenant_id=tenant_id)                  # temperature == 0
    result = filter_answer(memo, tenant_id=tenant_id)             # refuse_if no_citations
    return result  # refuses by default when ungrounded
```

### Rule 5: Rollback-Friendly

Each increment should be independently revertable:

- Additive changes (new files, new functions, new BotPolicy row, new lobe registry row) are easy to revert
- Modifications to existing code should be minimal and focused
- Alembic migrations must have a matching downgrade so they can roll back, and KB/Bot versions are append-only — never mutate a prior version in place
- Avoid deleting and replacing in the same commit — separate them

## Working with Agents

When directing an agent to implement incrementally:

```
"Let's implement Task 3 from the OpenSpec change.

Start with just the retrieve_kb ACL check and its pytest. Don't touch the
synthesize stage yet — that's the next increment.

Write the failing test first (per test-driven-development), then implement.
After implementing, run the resolver-selected gates for the touched package
(uv ruff/format/pyright/pytest, plus ci-free-gates.sh and openspec validate)
to verify nothing is broken. If DB-dependent tests skip, bring up pgvector
with docker compose -f docker-compose.dev.yml and set DATABASE_URL."
```

Be explicit about what's in scope and what's NOT in scope for each increment.

## Increment Checklist

After each increment, verify:

- [ ] The change does one thing and does it completely
- [ ] All existing tests in the touched package(s) still pass
- [ ] The touched package(s) still build/typecheck (`pyright` / `go build ./...` / `pnpm typecheck`)
- [ ] Lint/format are clean (`ruff check` + `ruff format --check` / `go vet` / `pnpm lint`)
- [ ] `bash benchmarks/ci-free-gates.sh` is green if a py/bench file changed
- [ ] `openspec validate "<change>" --strict` passes
- [ ] The new functionality works as expected (narrow test green)
- [ ] If inside an OpenSpec change, the matching `tasks.md` item is ticked
- [ ] The change is committed with a descriptive message

**Note:** Run each verification command after a change that could affect it. After a successful run, don't repeat the same command unless the code has changed since. A `DATABASE_URL`-skip is not a failure, but it's also not coverage — bring up pgvector for DB-dependent slices.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll test it all at the end" | Bugs compound. A bug in Slice 1 makes Slices 2-4 wrong. Test each slice. |
| "It's faster to do it all at once" | It *feels* faster until something breaks and you can't find which of 500 changed lines caused it. |
| "These changes are too small to commit separately" | Small commits are free. Large commits hide bugs and make rollbacks painful. |
| "I'll wire the new behavior into the live path now and gate it later" | If it isn't complete, keep it inert (inactive BotVersion / flag off) so the live retrieval and answer paths stay safe. |
| "This refactor is small enough to include" | Refactors mixed with features make both harder to review and debug. Split them (see `code-simplification`). |
| "I'll skip the downgrade migration for now" | Without a matching Alembic downgrade, the increment isn't rollback-friendly. Write it now. |
| "Let me run the gates again just to be sure" | After a successful run with no intervening edits, re-running adds nothing. Run them again after the next change. |

## Red Flags

- More than 100 lines written without running tests
- Multiple unrelated changes in a single increment
- "Let me just quickly add this too" scope expansion
- Skipping the test/verify step to move faster
- Build, typecheck, or tests broken between increments
- An Alembic migration without a matching downgrade, or an in-place mutation of an append-only version
- A code change sneaking into a slice that should only add a `BotPolicy`/lobe-registry row
- A slice that drops `tenant_id` from a query, cache key, or log line
- Large uncommitted changes accumulating
- Building abstractions before the third use case demands it
- Touching files outside the task scope "while I'm here"
- Re-running the same gate command with no intervening code change

## Verification

After completing all increments for a task:

- [ ] Each increment was individually tested and committed
- [ ] The resolver-selected gates pass for every touched package (uv/go/pnpm as applicable + `ci-free-gates.sh` + `openspec validate "<change>" --strict`)
- [ ] The feature works end-to-end as specified (exercise the benchmark ladder / `pnpm test:e2e` if it touches the answer pipeline or portal)
- [ ] No uncommitted changes remain

## MeKnow notes

- **OpenSpec lifecycle.** Start non-trivial work with `/opsx:propose`, quality-gate with `/opsx:spec`, merge the contract with `/opsx:spec-pr` (sync delta→canonical + the human-merged SPEC PR), then implement unit-by-unit with `/opsx:ship` (`ship-plan` groups the units, `ship-code` runs each Red→Green→one commit, ticking `tasks.md` as you go), which reconciles the delta vs canonical and `/opsx:archive` when the code PR merges. The autonomous `/opsx:ship` pipeline plans → implements → verifies → reconciles → opens the CODE PR; its Test(Red) phase pairs naturally with thin slices — one failing test, minimal code, repeat (see `test-driven-development`). All of this happens inside the `platform/` submodule.
- **Invariants to keep intact across slices:** multi-tenant by default (`tenant_id` on every table/query/cache key/log line; cross-tenant joins are bugs); citations mandatory (`filter` stage carries `refuse_if: no_citations`); `temperature == 0` on `synthesize`/`cite`/`filter`; ACL enforced server-side in `retrieve_kb` with caller identity inherited; versions append-only (`BotVersion`/`KBVersion` — new child with a parent pointer); compression invariant (only `memo_schema_ref`-shaped objects cross stage boundaries); MCP as the internal tool boundary (`kb.*`, embedded transport); no LangChain/LangGraph/Haystack/LlamaIndex; OpenAPI is the API contract (portal types generated); new bot/capability = new `BotVersion`/`BotPolicy` row + prompt file; new engine capability = a lobe/path registry row, never an interpreter branch.
- **Test-deliverable file patterns per slice:** `tests/test_<feature>.py` (pytest) for Python, `<feature>_test.go` (table-driven) for Go, `apps/portal/src/**/<feature>.test.ts(x)` (vitest) for TypeScript — in the touched package's language.
- **DB-dependent pytest** needs `DATABASE_URL`/`TEST_DATABASE_URL` pointing at a pgvector instance (local `docker compose -f docker-compose.dev.yml`; CI `pgvector/pgvector:pg16`). Absent a DB those tests skip (recorded, not failed) — bring up pgvector for DB-dependent slices.
