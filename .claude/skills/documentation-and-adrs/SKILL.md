---
name: documentation-and-adrs
description: Records decisions and documentation, adapted for the Mezon Mentor Bot ("MeKnow") platform. Use when making architectural decisions, changing the API contract (OpenAPI) or the BotPolicy/MCP boundary, shipping features, or recording context future engineers and agents need. Notes how OpenSpec design.md files serve as lightweight ADRs, that former RFCs live in docs/design/, and where docs/ lives.
tags: [docs]
---

# Documentation and ADRs

## Overview

Document decisions, not just code. The most valuable documentation captures the *why* — the context, constraints, and trade-offs that led to a decision. Code shows *what* was built; documentation explains *why it was built this way* and *what alternatives were considered*. This context is essential for future humans and agents working in the codebase.

## When to Use

- Making a significant architectural decision
- Choosing between competing approaches
- Adding or changing a public API surface (the OpenAPI contract, an MCP `kb.*` tool, a `BotPolicy` field)
- Shipping a feature that changes a capability's behavior contract
- Onboarding new team members (or agents) to the project
- When you find yourself explaining the same thing repeatedly

**When NOT to use:** Don't document obvious code. Don't add comments that restate what the code already says. Don't write docs for throwaway prototypes.

## Architecture Decision Records (ADRs)

ADRs capture the reasoning behind significant technical decisions. They're the
highest-value documentation you can write.

**In this repo, OpenSpec change `design.md` files already serve as lightweight,
per-change ADRs** — they record the problem, the chosen approach, and rejected
alternatives for that change. This is where new decisions go.

This repo has **just replaced its RFC system with OpenSpec**. The former RFCs
(`docs/design/`) are being preserved as design rationale under `docs/design/NNNN-<slug>.md`,
linked from each capability's `## Purpose`, and **`openspec/specs/<capability>/spec.md`
is now the canonical source of truth** for current behavior. So:

- A decision scoped to **one change** → that change's `design.md`.
- A **UI/visual decision** (component hierarchy, layout, interaction states) → that change's `ui.md`.
- A **durable, cross-cutting** decision future changes must respect → a doc under
  `docs/design/` (the former-RFC home), referenced from the relevant capability spec.

### When to Write a (durable) design doc / ADR under docs/design/

- A decision that constrains *future* OpenSpec changes (not just this one)
- Choosing a major dependency or boundary (`anthropic-sdk-python` directly — no
  LangChain/LangGraph; MCP as the internal tool boundary; `sentence-transformers`
  for local embeddings; MiniMax via the Anthropic-compatible endpoint)
- The multi-tenant data model and `tenant_id`-on-everything invariant
- The credential-encryption strategy (Fernet, `ingest_core.crypto`, decrypt
  server-side only)
- The retrieval-runtime / lobe-engine model (append-only versions, the compression
  invariant, `temperature == 0` on synthesize/cite/filter)
- The citations-mandatory / refuse contract
- Any decision that would be expensive to reverse

If a decision lives entirely within one change, capture it in that change's
`design.md` instead of a separate doc.

### Design-doc / ADR Template

Store durable design docs in `docs/design/` with sequential numbering (the
former-RFC convention):

```markdown
# 0007: Multi-tenant isolation keyed by tenant_id on every query and cache key

## Status
Accepted | Superseded by 00XX | Deprecated

## Date
2026-06-22

## Context
MeKnow is a multi-tenant RAG platform: many orgs share one deployment, and an
org must never see another org's documents, answers, or cache. Bolting tenancy on
per-feature would leak across joins and shared caches.

## Decision
Every table, query, cache key, and log line carries tenant_id. Cross-tenant joins
are bugs. Cache keys additionally include an ACL-cohort hash so two callers with
different KB permissions never share a cached answer. ACL is enforced server-side
inside retrieve_kb; the caller identity is inherited from the request, never
trusted from the model.

## Alternatives Considered

### Row-level security only, no tenant_id in cache keys
- Pros: DB enforces isolation at the row layer
- Cons: shared caches still leak across tenants/ACL cohorts
- Rejected: the cache is a real cross-tenant leak path; keys must be scoped

### Per-tenant database / schema
- Pros: hard isolation
- Cons: operationally heavy; breaks shared embeddings + golden-set tooling
- Rejected: tenant_id scoping gives the guarantee without the operational cost

## Consequences
- Cross-tenant isolation is gate-tested (benchmarks/gates/tenant-isolation-test.sh)
- Spec lives in openspec/specs/auth-and-tenancy/spec.md
```

### ADR / design-doc Lifecycle

```
PROPOSED → ACCEPTED → (SUPERSEDED or DEPRECATED)
```

- **Don't delete old design docs / RFCs.** They capture historical context — that's
  exactly why the former `docs/design/` are being preserved under `docs/design/`.
- When a decision changes, write a new doc that references and supersedes the old one.
- Cross-reference: link the doc from the relevant `openspec/specs/<capability>/spec.md`
  (`## Purpose`) and from the change `design.md` that introduced or revisited the decision.

## OpenSpec specs vs. design docs (this repo)

| Artifact | Captures | Lives in |
|---|---|---|
| `openspec/specs/<capability>/spec.md` | The *current* behavior contract (the source of truth) | `openspec/specs/` |
| change `design.md` | The reasoning for *this* change (a lightweight ADR) | `openspec/changes/<name>/design.md` |
| durable design doc (former RFC) | A cross-cutting decision future changes must respect | `docs/design/NNNN-<slug>.md` |

When behavior changes, update the spec via a change's delta + `/opsx:sync` — don't
let the spec drift from the code.

## Inline Documentation

### When to Comment

Comment the *why*, not the *what*:

```python
# BAD: Restates the code
# increment the counter
counter += 1

# GOOD: Explains a non-obvious invariant
# Cache key must include the ACL-cohort hash, not just tenant_id + query: two
# callers in the same tenant with different KB permissions must never share a
# cached answer, or retrieve_kb's server-side ACL is silently bypassed.
key = cache_key(tenant_id, query, acl_cohort_hash(caller))
```

### When NOT to Comment

```python
# Don't comment self-explanatory code
def total(items: list[LineItem]) -> int:
    return sum(it.price * it.qty for it in items)

# Don't leave TODO comments for things you should just do now
# TODO: add error handling  ← just handle the error

# Don't leave commented-out code  ← delete it, git has history
```

### Document Known Gotchas (this repo has several)

```python
# IMPORTANT: the caller identity is inherited from the request and passed to
# retrieve_kb explicitly; it is NEVER read from model output or tool arguments.
# ACL is enforced server-side here — the prompt is not a security boundary.
chunks = retrieve_kb(tenant_id=ctx.tenant_id, caller=ctx.caller, query=q)
```

Mirror the hard invariants already listed in `openspec/project.md` (citations
mandatory / `refuse_if: no_citations`, `temperature == 0` on
synthesize/cite/filter, append-only versions, the compression invariant — raw KB
chunks never leave a sub-agent) wherever the code enforces them.

## API Documentation

The **API contract is OpenAPI-generated**: the backend produces the spec and the
portal's types are *generated* from it — **never hand-write portal types**. Document
the *behavior* of an endpoint or tool at its source (backend route handler, MCP
`kb.*` tool) with a docstring; the wire shape comes from OpenAPI.

```python
def retrieve_kb(tenant_id: str, caller: Caller, query: str) -> list[CitedChunk]:
    """Retrieve grounded KB chunks for a query within one tenant.

    ACL is enforced server-side using `caller` (inherited from the request,
    never trusted from the model). Returns only chunks the caller is permitted
    to see, each carrying a citation; an answer-producing path that gets no
    permitted chunks MUST refuse rather than emit ungrounded claims.

    Raw chunks must not cross a sub-agent boundary — only memo_schema_ref-shaped
    objects do (the compression invariant).
    """
    ...
```

For HTTP routes, document the auth model inline (service JWT between backend and
workers; HMAC-signed inbound webhooks) and keep the canonical wire description in
OpenAPI and the matching `openspec/specs/` capability (`api-surface`).

## README & docs/ Structure

Durable cross-cutting docs live under `docs/`. Key entry points:

| Doc | Purpose |
|---|---|
| `README.md` | Quick start, gate commands, high-level overview |
| `docs/product/` | Product description, actors, use cases |
| `docs/openspec-workflow.md` | The spec-driven workflow and artifact formats |
| `docs/engineering/` | Engineering practice, toolchain map, conventions |
| `docs/design/` | Durable design rationale (the former RFCs, `NNNN-<slug>.md`) |
| `openspec/project.md` | Toolchain + hard invariants for agents and humans |

A good README covers quick start, the gate commands per toolchain, and an
architecture overview that links to `docs/` and `openspec/specs/`:

```markdown
## Gates (run from the owning package dir)
| Toolchain | Commands |
|-----------|----------|
| Python (`uv`) | `uv run ruff check .` · `uv run pyright` · `uv run python -m pytest -q` |
| Go (go 1.24) | `go build ./...` · `go vet ./...` · `go test -race ./...` |
| TypeScript (`apps/portal`) | `pnpm typecheck` · `pnpm lint` · `pnpm test` |
| Benchmarks | `bash benchmarks/ci-free-gates.sh` |
| Always | `openspec validate "<change>" --strict` |
```

## Changelog Maintenance

The repo keeps a root `CHANGELOG.md` in **Keep a Changelog** format — **one bullet
per shipped change**, generated by `/opsx:ship` when it ships an approved change.
You rarely hand-edit it; keep entries terse and user-facing:

```markdown
# Changelog

## [Unreleased]
### Added
- worker-retrieve enforces ACL-cohort cache keys for cross-tenant safety.

### Fixed
- Citation filter no longer emits an answer when retrieve_kb returns no permitted chunks.

### Changed
- Embeddings pinned to a single sentence-transformers model version per KBVersion.
```

## Documentation for Agents

Special consideration for AI agent context:

- **`openspec/project.md`** — toolchain map (the gate resolver's truth) and the hard
  invariants; keep it current and accurate.
- **`openspec/specs/`** — the behavior contract agents build against; update via delta + `/opsx:sync`.
- **change `design.md`** — lightweight ADRs that prevent re-deciding settled trade-offs.
- **Durable design docs (`docs/design/`)** — cross-cutting decisions (the preserved former RFCs) future changes must respect.
- **Inline gotchas** — prevent agents from falling into the repo's known traps (server-side ACL, citations-mandatory, the compression invariant, `tenant_id` on every query).

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "The code is self-documenting" | Code shows what. It doesn't show why, what alternatives were rejected, or which invariant it upholds. |
| "We'll write docs when the API stabilizes" | The spec/design *is* the first test of the design. Write it first via `/opsx:propose`. |
| "Nobody reads docs" | Agents read `openspec/project.md`, specs, and design docs. Future engineers do. Your 3-months-later self does. |
| "design.md is overhead" | A 10-minute design.md prevents a 2-hour re-litigation of a settled trade-off. |
| "Comments get outdated" | Comments on *why*/invariants are stable. Comments on *what* get outdated — write the former. |
| "I'll just update the code, not the spec" | The spec drifts and the next agent builds the wrong thing. Sync the spec. |
| "I'll just edit the portal types" | They're generated from OpenAPI. Hand-edits get clobbered — change the backend spec source instead. |

## Red Flags

- Architectural decisions with no written rationale (no design.md, no design doc)
- The code changed behavior but `openspec/specs/` was not updated
- Backend routes or MCP `kb.*` tools with no docstring; hand-written portal types
- README that doesn't explain how to run the gates (`uv run pytest`, `go test -race`, `pnpm test`)
- Commented-out code instead of deletion
- TODO comments that have lingered for weeks
- A cross-cutting decision (tenancy, secrets, retrieval-runtime) with no design doc under `docs/design/`
- `openspec/project.md` that no longer matches the code/toolchain

## MeKnow notes

- Durable docs live under `docs/` (`docs/engineering/`, `docs/design/`,
  `docs/product/`). The former RFCs (`docs/design/`) are being preserved as
  `docs/design/NNNN-<slug>.md` rationale, linked from each capability's `## Purpose`.
- OpenSpec change `design.md` files act as lightweight ADRs; reserve durable design
  docs in `docs/design/` for **cross-cutting** decisions future changes must respect.
- The behavior source of truth is `openspec/specs/<capability>/spec.md`; update it
  via a change's delta + `/opsx:sync`, not by editing the spec directly.
- The API contract is **OpenAPI-generated** — portal types are generated from it,
  never hand-written.
- `/opsx:ship` generates the root `CHANGELOG.md` entry (Keep a Changelog, one
  bullet per change) as part of apply → verify → sync → changelog → PR.
- Capture verification evidence under `openspec/changes/<name>/evidence/`.
- See the sibling `git-workflow-and-versioning` skill for commit/PR/CHANGELOG
  mechanics and `ci-cd-and-automation` for the quality gates these decisions ride through.

## Verification

After documenting:

- [ ] Significant decisions have a design.md (per-change) or a docs/design/ doc (cross-cutting)
- [ ] `openspec/specs/` reflects the new behavior (synced, not drifted)
- [ ] Backend routes / MCP `kb.*` tools have docstrings; portal types are generated (not hand-written)
- [ ] Known gotchas (server-side ACL, citations-mandatory, compression invariant, tenant_id) are documented inline where they matter
- [ ] README covers the toolchain gates and links to docs + specs
- [ ] No commented-out code remains
- [ ] `openspec/project.md` is current and accurate
