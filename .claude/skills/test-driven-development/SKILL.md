---
name: test-driven-development
description: Drives development with tests (polyglot + OpenSpec). Use when implementing any logic, fixing any bug, or changing any behavior. Use when you need to prove that code works, when a bug report arrives, or when you're about to modify existing functionality. Red-Green-Refactor with language-appropriate tests, the resolver-selected gates, and the `/opsx:ship` Test(Red) phase. Project specifics (test commands, file patterns, fixtures, invariants) come from `mzspec.config.json` and the project's `tdd` template.
tags: [test]
---

# Test-Driven Development

## Overview

Write a failing test before writing the code that makes it pass. For bug fixes, reproduce the bug with a test before attempting a fix. Tests are proof — "seems right" is not done. A codebase with good tests is an AI agent's superpower; a codebase without tests is a liability.

Concretely: a failing test **in the touched package's language** first, then the minimal code to make it pass, then the **resolver-selected gates** to keep the tree green. The exact test command, test-file naming, fixtures, and the invariants your tests must respect are **project-specific** — read them from this project's config and convention template, not from this skill (see [Project specifics](#project-specifics)).

## When to Use

- Implementing any new logic or behavior
- Fixing any bug (the Prove-It Pattern)
- Modifying existing functionality
- Adding edge case handling
- Any change that could break existing behavior

**When NOT to use:** Pure configuration changes, documentation updates, or static content changes that have no behavioral impact.

**Related:** For multi-file work, combine TDD with the `incremental-implementation` skill (one thin slice, tested, then expand). For end-to-end behavior, exercise the real path — including any project-level threshold/e2e gates the resolver emits.

## The TDD Cycle

```
    RED                GREEN              REFACTOR
 Write a test    Write minimal code    Clean up the
 that fails  ──→  to make it pass  ──→  implementation  ──→  (repeat)
      │                  │                    │
      ▼                  ▼                    ▼
   Test FAILS        Test PASSES       resolver gates PASS
```

### Step 1: RED — Write a Failing Test

Write the test first, in the language of the package you're touching. It must fail. A test that passes immediately proves nothing. Run the narrow test target and **confirm it fails**.

```python
# RED: a single, focused failing case (Python example shown — use the touched
# package's language and your project's test-file pattern).
import pytest
from billing.discount import apply_discount

@pytest.mark.parametrize(
    "name, price, pct, want",
    [
        ("ten percent off",   100.0, 10, 90.0),
        ("zero percent",      100.0, 0,  100.0),
        ("clamps above 100",  100.0, 150, 0.0),
    ],
)
def test_apply_discount(name, price, pct, want):
    assert apply_discount(price, pct) == want, name
```

The same shape applies in any language — a table/parametrized failing case beside the unit (e.g. table-driven `*_test.go`, or a `*.test.ts(x)` for a web app). Use your project's test-file pattern.

### Step 2: GREEN — Make It Pass

Write the minimum code to make the test pass. Don't over-engineer:

```python
# GREEN: minimal implementation — only what the test demands.
def apply_discount(price: float, pct: float) -> float:
    pct = max(0.0, min(pct, 100.0))
    return round(price * (1 - pct / 100), 2)
```

Re-run the narrow test target and confirm it now PASSES.

### Step 3: REFACTOR — Clean Up

With the test green, improve the code without changing behavior:

- Extract shared logic
- Improve naming
- Remove duplication
- Optimize only if necessary

Then confirm the **resolver-selected gates** are still green. Get the exact commands for the files you touched from the gate resolver — don't hand-type or guess them:

```bash
git diff --name-only <base>...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin
# → prints the exact per-toolchain gate commands (lint/type/test) from mzspec.config.json,
#   plus any project gates and `openspec validate "<change>" --strict`. Run each; each must exit 0.
```

Run the gates after every refactor step. (See the note on environment-dependent tests under "Verification" — a *skip* is not a failure.)

## The Prove-It Pattern (Bug Fixes)

When a bug is reported, **do not start by trying to fix it.** Start by writing a test that reproduces it.

```
Bug report arrives
       │
       ▼
  Write a test (in the touched package's language) that demonstrates the bug
       │
       ▼
  Run the narrow test target  →  FAILS (confirming the bug exists)
       │
       ▼
  Implement the fix
       │
       ▼
  Run the narrow test target  →  PASSES (proving the fix works)
       │
       ▼
  Resolver gates green (no regressions across the touched packages)
```

Writing the reproduction test *before* the fix — ideally without knowledge of the fix — makes it robust: it pins the behavior, not your implementation. If your project has a hard invariant the bug violated (data isolation, a refusal rule, determinism), the reproduction test must assert that boundary, drawn from your project's invariants (see below).

## The Test Pyramid

Invest testing effort according to the pyramid — most tests should be small and fast, with progressively fewer tests at higher levels:

```
          ╱╲
         ╱  ╲         E2E / product-threshold (~5%)
        ╱    ╲        full-flow + any threshold gates your project defines
       ╱──────╲
      ╱        ╲      Integration (~15%)
     ╱          ╲     crosses a real boundary (DB, API, another service)
    ╱────────────╲
   ╱              ╲   Unit (~80%)
  ╱                ╲  pure logic: parsers, transforms, policy/decision functions
 ╱──────────────────╲
```

**The Beyonce Rule:** If you liked it, you should have put a test on it. Refactors and migrations are not responsible for catching your bugs — your tests are.

### Test Sizes (Resource Model)

| Size | Constraints | Speed | Typical example |
|------|------------|-------|-----------------|
| **Small** | Single process, no I/O, no network, no DB | Milliseconds | pure logic, parsing, formatting, decision functions |
| **Medium** | Localhost only, no external services | Seconds | queries against a local DB, in-process integration, component tests |
| **Large** | External services / full pipeline | Minutes | the project's full-flow / threshold / e2e gates |

Small tests should make up the vast majority of your suite. They're fast, reliable, and easy to debug when they fail.

### Decision Guide

```
Is it pure logic with no side effects?
  → Small unit test (no DB, no network)

Does it cross a boundary (a DB/store, an API surface, another service)?
  → Medium test (integration against the real-ish boundary)

Is it a behavioral product guarantee your project defines (a threshold, an isolation rule)?
  → Large / e2e test — back it with the project gate that proves it
```

## Writing Good Tests

### Use Parametrized / Table-Driven Tests

Parametrized tests (pytest `@pytest.mark.parametrize`, Go table-driven, vitest `it.each`, etc.) are the idiom. Each row is a named case; failures point at the exact case.

```python
def test_seal_unseal_round_trip():
    cases = [("empty", ""), ("short", "hunter2"), ("unicode", "café-π-✓")]
    for name, plaintext in cases:
        sealed = seal(key, plaintext.encode())
        assert unseal(key, sealed).decode() == plaintext, name
```

### Test State, Not Interactions

Assert on the *outcome* of an operation, not on which internal functions were called. Tests that verify call sequences break when you refactor, even if behavior is unchanged.

```python
# Good: tests what the function does (state-based)
def test_normalize_is_idempotent():
    once = normalize(raw)
    assert normalize(once) == once
```

If a function is contractually deterministic, assert the deterministic outcome — don't assert "the client was called with these args."

### DAMP Over DRY in Tests

In production code, DRY is usually right. In tests, **DAMP (Descriptive And Meaningful Phrases)** is better. Each test should read like a specification without forcing the reader to trace shared helpers. Duplication in tests is acceptable when it makes each case independently understandable. (Parametrized rows are the sweet spot: shared body, self-describing data.)

### Prefer Real Implementations Over Mocks

Use the simplest test double that does the job. The more real code your tests exercise, the more confidence they give.

```
Preference order (most to least preferred):
1. Real implementation  → the real dependency (highest confidence)
2. Fake                 → in-memory store / fake clock / lightweight local substitute
3. Stub                 → returns canned, deterministic responses
4. Mock (interaction)   → verifies method calls — use sparingly
```

**Use stubs/mocks only when** the real dependency is too slow, non-deterministic, or has side effects you can't control (an external API, wall-clock time, a paid/LLM endpoint). Stub those with deterministic responses in unit tests; reserve real calls for the higher-tier gates.

### Use Arrange-Act-Assert, One Assertion Per Concept, Descriptive Names

```python
# Good: reads like a specification
def test_parse_rejects_trailing_comma(): ...
def test_export_preserves_unicode(): ...

# Bad: vague
def test_parse(): ...
def test_works(): ...
```

Split behaviors into separate cases (or parametrized rows) rather than cramming several validations into one test.

## Test Anti-Patterns to Avoid

| Anti-Pattern | Problem | Fix |
|---|---|---|
| Testing implementation details | Tests break on refactor even when behavior is unchanged | Test inputs and outputs, not internal structure |
| Flaky / nondeterministic tests | Erode trust; an uncontrolled clock/network/LLM call is inherently flaky | Stub the nondeterministic dependency; deterministic clocks; isolate state |
| Testing the framework / SDK | Wastes effort on third-party behavior | Only test YOUR code, not the libraries it uses |
| No isolation between cases | Shared mutable state hides bugs and causes order-dependence | Fresh fixtures per case; seed independent data |
| Mocking everything | Tests pass while production breaks | Real > fakes > stubs > mocks |
| Skipping/disabling tests to go green | Hides the failure | Fix the cause, or record an explicit, justified skip |

If your project has a specific must-respect invariant (data isolation, a refusal rule, append-only history), add a row for it — see your project's `tdd` template.

## When to Use Subagents for Testing

For complex bug fixes, spawn a subagent to write the reproduction test (in the touched package's language) and confirm it FAILS with the current code — then the main agent implements the fix and confirms it passes plus the resolver gates are green. Writing the test without knowledge of the fix makes it more robust.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll write tests after the code works" | You won't. And after-the-fact tests test implementation, not behavior. |
| "This is too simple to test" | Simple code gets complicated. The test documents the expected behavior. |
| "Tests slow me down" | They slow you down now and speed you up on every later change. |
| "I tested it manually" | Manual testing doesn't persist. Tomorrow's change breaks it silently. |
| "Env-dependent tests are skipping, so I'm fine" | A skip is not a pass. Provide the dependency (DB/service) the project's `tdd` template documents. |
| "It's just a prototype" | Prototypes become production. Tests from day one prevent test debt. |
| "Let me run the gates again to be sure" | After a clean run with no intervening edits, re-running adds nothing. |

## Red Flags

- Writing code without any corresponding test in that package's language
- Tests that pass on the first run (they may not be testing what you think)
- "All tests pass" but no tests were actually run
- Bug fixes without reproduction tests
- Tests asserting on internal call order instead of behavior
- A nondeterministic test (uncontrolled clock/network/model) with no stub
- A test that ignores a project invariant the change touches
- Skipping or disabling tests to make the suite pass
- Re-running the gates with no intervening code change

## Verification

After completing any implementation:

- [ ] Every new behavior has a corresponding test in the touched package's language (parametrized/table-driven where practical)
- [ ] The **resolver-selected gates** pass (the exact commands from `gate-resolver.js` + `openspec validate "<change>" --strict`)
- [ ] Bug fixes include a reproduction test that failed before the fix
- [ ] Test names describe the behavior being verified
- [ ] No tests were skipped or disabled to get green
- [ ] Environment-dependent tests actually ran against their dependency (a skip without it is recorded, not coverage)
- [ ] Any behavioral guarantee is backed by the relevant project gate
- [ ] Coverage hasn't decreased (use your toolchain's coverage gate)

**Note:** Run a gate after a change that could affect its result. After a clean run, don't repeat the same command unless the code has changed since.

## Project specifics

This skill is the **generic methodology**. The concrete, project-specific facts live outside it — read them, don't assume:

- **Test commands per toolchain** → `mzspec.config.json` → `toolchains.<tc>.gates` (the resolver emits these for the files you touched). Don't hardcode `pytest`/`go test`/`pnpm test` — use what the config says.
- **Invariants your tests must respect** → `mzspec.config.json` → `invariants` (e.g. data-isolation, refusal, determinism, append-only). A reproduction/edge test should assert the relevant one.
- **Test-file patterns, frameworks, fixtures, DB/service setup, and any threshold gates** → the project's **`tdd` convention template** at `<templatesDir>/tdd/TEMPLATE.md` (default `openspec/templates/tdd/`). View it with `node .claude/workflows/lib/templates.js show tdd`. If absent, follow your repo's existing conventions and propose authoring one (`/opsx:template-create`).
- **OpenSpec lifecycle.** Non-trivial work is spec-driven; the `/opsx:ship` pipeline runs a **Test(Red) phase before Implement** — failing tests first, then the implementation makes them green. Test evidence (output + coverage) is captured under `openspec/changes/<name>/evidence/`.
