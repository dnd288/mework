---
name: tdd
description: This project's TDD conventions — test commands, test-file patterns, fixtures, and invariants the test-driven-development skill defers to. (Starter — fill in for your project.)
---

# TDD conventions for this project

This is a **convention guide** (not a planning playbook). The generic `test-driven-development`
skill carries the methodology; this template holds the project-specific facts it defers to. Fill in
the placeholders for your repo, or `/opsx:template-remove tdd` if you don't need one (the skill then
falls back to `mzspec.config.json` + your repo's existing conventions).

## Test command per toolchain

Use the resolver-emitted gates (from `mzspec.config.json` → `toolchains.<tc>.gates`); don't hand-type.
List the narrow "run one test file" form here for the Red/Green inner loop:

- `<toolchain>` — narrow: `<e.g. uv --directory {dir} run python -m pytest -q tests/test_x.py>`
- `<toolchain>` — narrow: `<e.g. (cd {dir} && go test -race -run TestX ./...)>`
- `<toolchain>` — narrow: `<e.g. (cd apps/web && pnpm test path/to/x.test.ts)>`

## Test-file patterns (where the failing test goes)

- `<lang>`: `<e.g. tests/test_<feature>.py>`
- `<lang>`: `<e.g. <feature>_test.go (table-driven)>`
- `<lang>`: `<e.g. src/**/<feature>.test.ts(x)>`

## Fixtures / golden data

- `<e.g. tests/fixtures/...>` — what it is and how to seed it.

## Environment-dependent tests

- `<e.g. DB tests need DATABASE_URL/TEST_DATABASE_URL pointing at ...; bring it up with ...>`
  A skip without the dependency is recorded, not coverage.

## Invariants a test must respect

Pull these from `mzspec.config.json` → `invariants`, and note how each is asserted in tests:

- `<invariant>` → `<how a test asserts/guards it>`

## Behavioral / threshold gates (E2E tier)

- `<e.g. bash <project>/gates/<x>.sh>` — the product guarantee it proves and its threshold.
