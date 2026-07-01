# Engineering practice skills (SDD + TDD)

This repo pairs its **spec backbone** (OpenSpec — see
[openspec-workflow.md](openspec-workflow.md)) with a layer of **engineering-practice
skills** adapted from the MIT-licensed
[addyosmani/agent-skills](https://github.com/addyosmani/agent-skills) (see
[`.claude/skills/ATTRIBUTION.md`](../.claude/skills/ATTRIBUTION.md)). OpenSpec answers
*what* to build and tracks the change; the skills encode *how* to build it well —
spec discipline, test-first development, review, simplification, git/CI hygiene.

The skills are plain `SKILL.md` files under `.claude/skills/`; Claude Code surfaces
each by its `description` and you (or a workflow) invoke the relevant one per phase.
They are **tracked in the repo** (see the `.gitignore` carve-out) so the whole team
gets them on clone.

## The lifecycle: spec → design → test → implement → verify → ship

| Stage | OpenSpec backbone | Skill(s) applied | Evidence |
|-------|-------------------|------------------|----------|
| **Spec** | `/opsx:propose` → `proposal.md` + delta specs (single-pass draft) | `spec-driven-development` (maps onto OpenSpec — no competing PRD) | — |
| **Spec-review** | `/opsx:spec` → cross-validate 6 axes → revise until clean | `spec-review-and-quality` | `review/REVIEW.md` |
| **Design** | change `design.md` | `planning-and-task-breakdown` | — |
| **Test (Red)** | acceptance criteria in `tasks.md` | `test-driven-development` | failing `*_test.go` committed |
| **Implement (Green)** | `/opsx:apply` ticks tasks | `incremental-implementation`, `code-simplification` | tasks ticked |
| **Verify** | `make vet` / `make test` gates | `debugging-and-error-recovery`, `code-review-and-quality`, `security-and-hardening` | `evidence/` (tests, coverage, gates) |
| **Ship** | `/opsx:ship` → PR; `/opsx:archive` post-merge | `git-workflow-and-versioning`, `ci-cd-and-automation`, `documentation-and-adrs` | `CHANGELOG.md` entry, PR body |
| **Review-response** | `/opsx:address-review` | reuses `test-driven-development` + `code-review-and-quality` | reply/resolve threads via `gh` |

`using-agent-skills` is the meta-router: it maps incoming work onto this table and
enforces six non-negotiable operating rules — **surface assumptions, manage
confusion, push back when warranted, enforce simplicity, scope discipline, and
verify-don't-assume (evidence required).**

## Test-first, with evidence

The lifecycle is **TDD-first**: write a failing Go test that pins the new behavior,
make it pass with the minimal change, then refactor while the gates stay green
(`go build ./...`, `make vet`, `make test`). DB-backed tests skip unless
`TEST_DATABASE_URL` is set (`make test-db` starts Postgres) — a skip is not a
failure.

Every shipped change leaves a durable audit trail in
**`openspec/changes/<name>/evidence/`**:
- `gates.md` — which verify gates ran and their outcomes, coverage total, Red status, repair count.
- `test-results.md` — `go test` summary (pass/fail/skip counts).
- `coverage.txt` — `go tool cover -func` summary.

The evidence directory moves into the archive with the change and is linked from the
PR body, so a reviewer can see the proof without re-running anything.

## Automation that uses these skills

- **`/opsx:spec`** (`.claude/workflows/spec-change.js`) — quality-gates a change's
  spec before code: fans out one read-only critic per spec-review axis **in
  parallel**, then revises Blocker/Required findings and re-runs `openspec validate
  --strict` until clean. Writes `openspec/changes/<name>/review/REVIEW.md`. This is
  what makes `/opsx:ship-plan`'s test tasks (drawn **from the delta-spec scenarios**)
  concrete and complete — clean scenarios in, complete features out.
- **`/opsx:ship-plan`** (`.claude/workflows/ship-plan.js`) — writes a reviewable
  handoff under `.handoff/<change>/`: each change task becomes a **test** task + a
  **code** task. **`/opsx:ship-code`** (`.claude/workflows/ship-code.js`) executes it
  test-first — per change task: Red → Green → **one commit** (the failing test + its
  implementation) — then Verify → Evidence → Sync → Changelog → PR. **`/opsx:ship`**
  orchestrates both with a review gate. Stops at PR opened; `/opsx:archive` finalizes.
- **`/opsx:address-review`** (`.claude/workflows/address-review.js`) — fetches PR
  review comments via `gh`, makes test-first fixes, commits/pushes, replies to and
  resolves threads, and re-requests review.
- **CI** (`.github/workflows/ci.yml`) — the same gates (`go build`, `make vet`,
  `make test`) on every push/PR, with a Postgres service container so DB tests run.

## The skills: 12 vendored + 1 repo-authored

The 12 vendored skills:

`using-agent-skills` · `spec-driven-development` · `planning-and-task-breakdown` ·
`test-driven-development` · `incremental-implementation` ·
`debugging-and-error-recovery` · `code-review-and-quality` · `code-simplification` ·
`security-and-hardening` · `git-workflow-and-versioning` · `ci-cd-and-automation` ·
`documentation-and-adrs`.

Plus one **repo-authored** skill — **`spec-review-and-quality`** — the spec-layer
counterpart of `code-review-and-quality`. It defines the six spec-review axes
(Structure/validity · Clarity/KISS · Testability · Minimality/YAGNI ·
Consistency/DRY · Completeness) and powers the `/opsx:spec` workflow.

Each was localized to Go + `make` + the OpenSpec lifecycle and the repo's invariants
(prompts over stdin, immutable terminal job states, provider-agnostic schema,
AES-256-GCM-sealed credentials, `-p 1` DB tests, `0600`/`0700` file perms). Skills
not relevant to this backend daemon (frontend-ui, browser-testing, performance,
observability, deprecation/migration, shipping-and-launch, the define-phase
interview/idea skills) were intentionally left upstream.
