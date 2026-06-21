# Spec-Driven Development with OpenSpec

This project uses **[OpenSpec](https://github.com/Fission-AI/OpenSpec)** (CLI
v1.4.x) for spec-driven development. The rule of thumb: **non-trivial changes
start with a spec/change proposal, not with code.**

## Why

- Specs in `openspec/specs/` are the durable description of *what the system
  does*. Changes in `openspec/changes/` describe *what a particular piece of work
  alters*, and become the audit trail once archived.
- An AI agent (or a new teammate) can read the relevant spec to understand a
  subsystem before touching it, and the spec is updated as part of the change —
  so docs and behavior don't drift.

## Layout

```
openspec/
├── specs/                       # main/canonical specs (current behavior)
│   └── <capability>/spec.md
└── changes/
    ├── cNNNN-<slug>/            # an in-progress change + its artifacts
    └── archive/
        └── YYYY-MM-DD-cNNNN-<slug>/   # completed changes
```

Baseline capabilities already specced in `openspec/specs/`:
`provider-gateway`, `webhook-pipeline`, `job-queue`, `rest-writeback`,
`daemon-runtime`, `cli`, `auth-and-secrets`.

## Change naming & ordering

Every change directory is named **`cNNNN-<kebab-slug>`**, where `NNNN` is a 4-digit,
zero-padded sequence number that encodes **apply/dependency order**. Lower numbers
land first.

```
openspec/changes/c0001-repo-restructure/
openspec/changes/c0002-message-bus/
openspec/changes/c0003-agent-catalog/
openspec/changes/c0004-agent-runner/
openspec/changes/c0005-sandbox-runtime/
```

> **Why the leading `c`?** OpenSpec **requires a change name to start with a
> letter** — `openspec new change`, `openspec status --change`, and
> `openspec instructions --change` all reject a name that begins with a digit. The
> `c` (for *change*) prefixes the number so the name is CLI-valid while the
> zero-padded digits still sort by order.

- **Pick the next number** when proposing: take the highest existing number and add
  one (`/opsx:propose` does this automatically). Start at `c0001`.
  ```bash
  ls -1d openspec/changes/c[0-9]* 2>/dev/null | sed -E 's#.*/c([0-9]+)-.*#\1#' | sort -n | tail -1
  ```
- **Inserting before existing work**: choose an earlier free number; if none is
  free, renumber the later changes (they are plain directories — `mv` is safe; the
  change id is derived from the directory name, not stored in `.openspec.yaml`).
- The number is **ordering metadata only** — it is not the capability name. A change
  `c0002-message-bus` may add a capability spec named `message-bus`; capability names
  in `openspec/specs/` stay un-prefixed.
- The prefix is **preserved through archival**: archived changes become
  `archive/YYYY-MM-DD-cNNNN-<slug>/`.

## The lifecycle

```
/opsx:explore      think through the idea — NO code is written in this mode.
        │          (may capture insights into artifacts)
        ▼
/opsx:propose      create a change (named cNNNN-<slug>, next in order) and generate
  "<slug>"           its artifacts in dependency order (single-pass DRAFT):
                     proposal.md   → what & why
                     specs/...     → DELTA specs (ADDED/MODIFIED/REMOVED/RENAMED)
                     design.md     → how
                     tasks.md      → implementation steps
        │
        ▼
/opsx:spec         cross-validate the draft across the 6 spec-review axes (one critic
  "<slug>"           per axis, in parallel) → revise Blocker/Required findings until
                   clean → write review/REVIEW.md. Quality-gates the spec before code.
        │
        ▼
/opsx:apply        implement the tasks; tick - [ ] → - [x] in tasks.md as you go.
        │
        ▼
/opsx:sync         merge the change's delta specs into openspec/specs/.
        │          (optional/standalone; the change stays active)
        ▼
/opsx:archive      verify artifacts + tasks; offer to sync; then move the change
                   to openspec/changes/archive/YYYY-MM-DD-cNNNN-<slug>/.
```

Once a change is **approved**, the manual apply → sync steps can be collapsed into
two autonomous runs (plan, then execute):

```
/opsx:ship-plan    write a reviewable handoff under .handoff/<slug>/ — each change task
  "<slug>"           becomes a TEST task + a CODE task (the test plan + the impl plan).
        │
        ▼ (review/edit the handoff)
/opsx:ship-code    for each change task: Red (failing test) → Green (impl) → ONE commit;
  "<slug>"           then verify → evidence → sync → changelog → push → open PR.
                   (/opsx:ship runs both with a review gate between them)
```

`/opsx:ship` orchestrates plan→gate→code; see [§ Autonomous ship-to-PR](#autonomous-ship-to-pr).

The commands are available as `/opsx:*` slash commands and as auto-triggered
skills (e.g. "start implementing the change" triggers apply). Both drive the same
`openspec` CLI; the CLI is the source of truth for paths — the commands read
`openspec status --change "<name>" --json` rather than assuming repo-local paths.

> The workflow is **fluid, not phase-locked**: you can apply before every
> artifact is done, interleave exploration, and revise artifacts mid-flight if
> implementation reveals a design issue.

## Spec format

### Main spec (`openspec/specs/<capability>/spec.md`)

```markdown
# <Capability> Specification

## Purpose
<one paragraph: what this capability is and why>

## Requirements

### Requirement: <name>
The system SHALL <behavior>.   <!-- SHALL/MUST must appear in the body -->

#### Scenario: <name>
- **WHEN** <condition>
- **THEN** <expected outcome>
```

Rules enforced by `openspec validate`:
- A spec MUST have a `## Purpose` and a `## Requirements` section.
- Every `### Requirement:` MUST contain `SHALL` or `MUST` **in the body** (the
  line after the header), and MUST have **at least one** `#### Scenario:`.

### Delta spec (inside a change)

A change's `specs/<capability>/spec.md` is a **delta**, not a replacement:

```markdown
## ADDED Requirements
### Requirement: <new requirement>
...

## MODIFIED Requirements
### Requirement: <existing requirement>
...   <!-- include the full updated requirement, even to add one scenario -->

## REMOVED Requirements
### Requirement: <requirement to remove>

## RENAMED Requirements
- FROM: `### Requirement: <old>`
- TO: `### Requirement: <new>`
```

The delta expresses *intent*. `/opsx:sync` (or `/opsx:archive`) merges it into the
main spec. To add a scenario to an existing requirement, put that requirement
under `## MODIFIED Requirements` with the scenario included — don't hand-edit the
main spec.

## Spec quality — the 6 review axes

`openspec validate` is a **structural** gate; it does not judge whether a spec is
*good*. The `/opsx:spec` workflow and the `spec-review-and-quality` skill hold every
spec to six axes (a bad spec produces a partial feature — `/opsx:ship` derives its
tests from the spec's scenarios):

1. **Structure & validity** *(mechanical)* — Purpose + Requirements; SHALL/MUST in
   the body line; ≥1 Scenario each; delta uses ADDED/MODIFIED/REMOVED/RENAMED with
   the full requirement on MODIFY; `openspec validate --strict` passes.
2. **Clarity & KISS** — one requirement = one behavior; no "and…and…" packing.
3. **Testability** — every scenario decidable with concrete literals; no soft
   `MAY`/"to the extent of" in an intended-behavior THEN; edge/negative cases where
   they matter.
4. **Minimality & YAGNI** — spec only in-scope/built behavior; future options live
   in `design.md`, not as requirements; spec behavior, not process.
5. **Consistency & DRY** — each behavior defined once (reference, don't restate);
   canonical glossary terms; aligned with the `CLAUDE.md` invariants.
6. **Completeness ("not partials")** — every proposal claim → a requirement; every
   requirement → ≥1 scenario **and** covering task(s); `design.md` records the
   non-trivial decisions.

Findings are labelled **Blocker** → **Required** → **Nit** → **FYI**; the revise
loop exits only when no Blocker/Required remains.

## Terminology / glossary

New and revised specs use the **canonical** terms below; baseline specs keep their
legacy terms until a change supersedes them (rename via `## RENAMED Requirements`).
Never mix a canonical term and its legacy synonym for the same concept within one
spec.

| Canonical | Legacy / avoid | Meaning |
|-----------|----------------|---------|
| **hub** | server (as broker) | central server: registry, broker, catalog, orchestrator, sessions |
| **runner** | runtime, daemon, client | the enrolled local worker (subscribe → pull → run → report) |
| **dispatch** | job, claim | a unit of work the hub publishes to a runner/session topic |
| **session** | — | a live runner↔hub association over the SSE channel |
| **grant** | — | a scoped, least-privilege permission set carried by a dispatch |
| **sandbox** | workdir | the isolated runtime that runs one agent |
| **agent** | profile (static) | a versioned, pullable unit of instruction/behaviour |

### Autonomous spec authoring (`/opsx:spec`)

`/opsx:spec "<change>"` runs the quality pass autonomously, the authoring
counterpart of `/opsx:ship`. It is a **JS Workflow**
(`.claude/workflows/spec-change.js`) launched by `.claude/commands/opsx/spec.md`
after an `AskUserQuestion` gate. Phases: **Preflight** (load the change, or
scaffold+draft a new one with the next `cNNNN-` number) → **Cross-validate** (one
read-only critic agent **per axis, in parallel**) → **Revise** (fix Blocker/Required
findings, re-run `openspec validate --strict`, loop ≤ `maxRevisions`) → **Report**
(`openspec/changes/<change>/review/REVIEW.md` + an approve/revise verdict). Pass
`--dry-run` to review only (no edits). Run it between `/opsx:propose` and
`/opsx:apply`/`/opsx:ship`.

## Worked example (this repo)

Adding a second provider adapter (say, GitHub Issues):

1. `/opsx:explore` — sketch how a GitHub adapter maps onto the
   `provider-gateway` and `webhook-pipeline` capabilities.
2. `/opsx:propose "github-provider-adapter"` — picks the next order number and
   creates `openspec/changes/c0006-github-provider-adapter/` (after the existing
   `c0001`–`c0005`), generating a proposal, delta specs under `webhook-pipeline` /
   `provider-gateway` / `rest-writeback`, a design, and tasks.
3. `/opsx:spec c0006-github-provider-adapter` — cross-validate the draft across the
   6 axes and revise until the verdict is **approve** (writes `review/REVIEW.md`).
4. `/opsx:apply` — implement `internal/server/provider/github/`, register the
   adapter, add tests; tick off tasks.
5. `/opsx:archive` — sync the deltas into `openspec/specs/` and archive the change.

<a id="autonomous-ship-to-pr"></a>
## Autonomous ship-to-PR (`/opsx:ship-plan` → `/opsx:ship-code`)

Once a change's spec is **approved**, two JS Workflows carry it up to a PR. They are
split so the plan is reviewable before any code is written, and so each change task
lands as its own test-first commit. `/opsx:ship` orchestrates both with a review
gate between them. (The orchestration idiom is borrowed from the presale
`plan-tasks.js`/`run-tasks.js`.)

### 1. `ship-plan` (`.claude/workflows/ship-plan.js`) — write the handoff

Reads the approved change and writes a reviewable **handoff** under
`.handoff/<slug>/` (gitignored — local execution scaffolding):

```
.handoff/<slug>/
  README.md          # shared context: links proposal/design/tasks/delta-specs + task index
  plan.json          # the index (one entry per handoff task)
  tasks/
    01-a-test.md     # Red task for change task 01 — the test plan
    01-b-code.md     # Green task for change task 01 — the impl plan
    02-a-test.md
    02-b-code.md
```

For **each** task in the change's `tasks.md` it emits **two** handoff tasks — a
**test** task (which `*_test.go`, the assertions drawn from the delta-spec scenarios)
and a **code** task (the production `.go`), with `code depends_on test` and a shared
`pair`. No branch, no code. Idempotent: re-planning preserves `done` tasks. Review or
hand-edit the task files before running `ship-code`.

### 2. `ship-code` (`.claude/workflows/ship-code.js`) — execute it, test-first

Each phase is a sub-agent with a strict schema:

| Phase | What it does | Stops the run if… |
|-------|--------------|-------------------|
| **Preflight** | tools+**toolchain check** (go ≥1.25), `openspec validate`, `git status` clean, branch `feat/<slug>`, **load `.handoff/<slug>/plan.json`** | tool/version wrong, validation fails, tree dirty, or no handoff |
| **Implement** | per change task (pair): **Red** (write failing test, confirm it fails) → **Green** (minimal impl, confirm pass, tick `tasks.md`) → **one commit** containing both | a pair can't go Red/Green after repairs |
| **Verify** | `go build` + `make vet` + `make test` + coverage + `openspec validate`; deterministic (exit-0), repair loop (≤2) | gates still fail after repairs |
| **Evidence** | writes `openspec/changes/<slug>/evidence/` (`gates.md`, `test-results.md`, `coverage.txt`) | — |
| **Sync** | merges delta specs into `openspec/specs/` via `openspec-sync-specs` | — |
| **Changelog** | prepends a *Keep a Changelog* bullet to `CHANGELOG.md` + a final **chore commit** | — |
| **PR** | `git push` → `gh pr create`/update against `main` (body links the evidence) | budget reserve hit (commits kept local) |

> **Local ship path (`/opsx:ship --local`)** — see [Local ship path](#local-ship-path-opsxship---local) below. With `--local=true`, the run **replaces** the `PR` phase with: **Local review** → **Merge** (`git merge --<strategy> feat/<change>` into `<base>`, local-only) → **Post-merge verify** → **Archive** → optional **Tag** → **Cleanup** (chore commit + `branch -D` + optional `git push origin <base>` + `evidence/post-merge.md`). No `gh`, no remote push unless `--push-main`.

So an N-task change yields **N feature commits** (each = the failing test + its
implementation) plus **one chore commit** (evidence/changelog/spec sync).

Boundaries and knobs:
- **Stops at PR opened** — no auto-merge. After merge run `/opsx:archive <slug>`.
- **One commit per change task** = the Red test and the Green implementation together.
- **`dryRun`** (`--dry-run`) makes the per-task commits locally but skips push + PR.
  `--only <pair>` runs a single pair; `--retry-blocked` re-runs blocked pairs.
- Verify is **deterministic-first**: a gate passes only on exit 0. DB-backed tests
  skip unless `TEST_DATABASE_URL` is set; the run records gates + coverage.
- The Preflight **toolchain check** fails fast if `go` is older than 1.25 (e.g. an
  ancient `go` earlier in PATH) or `openspec`/`gh` are missing.
- The scripts can't read the system clock, so the command passes `args.date`.

## Local ship path (`/opsx:ship --local`)

The default ship flow ends at **PR opened**; the local path ends at **locally-
merged-and-archived `main`**, with no `gh` and no remote push (unless
`--push-main`).

When `/opsx:ship` runs with the **Local merge** path:

1. **Branch** `feat/<change>` → per-pair **Red → Green → one commit** (same as
   the remote path).
2. **Verify** (`make vet`/`make test` + coverage + `openspec validate`).
3. **Local review** — read-only audit of the diff vs `base` against the
   `code-review-and-quality` + `security-and-hardening` skills (gated on
   `--no-review`). Findings: `blocker` / `required` / `nit` / `fyi`. A `fail`
   verdict halts before merge; the user fixes locally on `feat/<change>` and
   re-runs (already-done pairs are skipped).
4. **Pre-merge evidence** (`gates.md`, `test-results.md`, `coverage.txt`).
5. **Merge** — `git switch <base> && git merge --<strategy> feat/<change>` with
   `--strategy={squash,no-ff,ff-only}` (default `squash`). The merge commit is
   a Conventional Commit signed off by the agent; **never** `git add -A`,
   **never** auto-resolves conflicts (the merge is refused on conflict and
   surfaced to the human).
6. **Post-merge verify** — re-runs all gates on `<base>` post-merge to catch
   merge-time breakage (e.g. an `openspec/` change-dir being touched).
7. **Sync** delta specs into `openspec/specs/<cap>/spec.md` (existing).
8. **Archive** — `mv openspec/changes/<change>/` →
   `openspec/changes/archive/YYYY-MM-DD-<change>/`. A row is appended to
   `openspec/changes/archive/INDEX.md` (created if missing).
9. **Tag** (optional, `--bump={patch,minor,major}`) — bumps the prior tag (or
   `0.0.0` if none) and creates an annotated `vX.Y.Z` on `<base>`.
10. **Cleanup** — one chore commit (evidence + sync + changelog +
    `evidence/post-merge.md`) → `git branch -D feat/<change>` → optional
    `git push origin <base>` (and the tag) when `--push-main`. Otherwise
    `pushed=false` and `pushReason="fully local — noPushMain=true"`.

Flags honored on the local path: `--local` (required), `--base=<branch>`
(default `main`), `--mergeStrategy={squash,no-ff,ff-only}`,
`--bump={patch,minor,major}`, `--no-push-main` (default true),
`--no-archive` (default false), `--no-review` (default false),
`--keep-branch` (default false), `--dry-run` (refuses merge, stops at
Verify), `--only=<pair>`, `--retry-blocked`, `--reserveTokens=<n>`,
`--max-repairs=<n>`, `--force` (ignores dirty-main / wrong-branch warnings
but never overrides a blocker review finding or a failing gate).

Review feedback on the local path is handled by the user on the
`feat/<change>` branch before re-running the ship — there is no separate
`address-review` workflow because there are no GitHub review threads.

## Batch ship (`/opsx:ship-all`)

The `/opsx:ship --local` flow ships **one change at a time**. `/opsx:ship-all`
walks **every ACTIVE OpenSpec change** through the full pipeline in one go,
**fully automatically — no confirmation, no per-change prompts**, auto-deciding
per change what to do, and halting on first failure with full progress. Each
change keeps the full workflow — branch, per-task commits, verify, review — and
merges into `main` **locally instead of opening a PR**.

**Branch + execution model.** The orchestrator owns branch creation; `ship-code`
owns the implementation. For every change that ships, it first runs **branch-prep**
(from a clean `main`, create or reuse `feat/<change>`), then invokes the nested
workflows via the lowercase `workflow()` helper — `workflow('ship-plan', …)` then
`workflow('ship-code', …)`. (It does **not** spawn an agent that calls `Workflow()`;
workflow-spawned subagents have no `Workflow()` tool — that was the original break.)
`ship-plan` groups the change into a **few test-first units** (aim 1–4; each may span
several files) — **not** one unit per tasks.md line — and `ship-code` runs each unit
Red→Green→**one commit per unit**. There is **no standalone `/opsx:apply`** step (a
bulk apply would write uncommitted code and trip `ship-code`'s clean-tree preflight).
After review, each change **merges into `main` locally** (so the next dependency-
ordered change builds on it) **and opens a PR** (`gh pr create`) for the record/human
review — result: the project on `main` plus one PR per change.

> The fine-grained "pair per OpenSpec task" handoff (28 files / 14 commits / ~2h for
> a 14-task change) proved too slow; the coarse "few units" model is the fix.

**Per-change mode** (decided from `openspec status --change <c> --json`):

| `openspec status` | Mode | Steps |
|---|---|---|
| full artifacts, `.openspec.yaml` present, 0 tasks done | `apply+ship` | branch-prep → `ship-plan` (few units) → `ship-code --local --openPr` (implements each unit test-first → review → local merge → archive → open PR) |
| full artifacts, all tasks `[x]`, no evidence/ | `spec+ship` | branch-prep → (`spec-change` quality pass, skipped by default in batch) → `ship-plan` → `ship-code --local --openPr` |
| full artifacts, all tasks `[x]`, evidence/ present | `ship-only` | branch-prep → `ship-plan` (0 units) → `ship-code --local --openPr` (verifies, review, merges, archives, open PR) |
| missing `.openspec.yaml` (scaffolding-only, e.g. c0014a/b/c) | `repair+ship` | `openspec new change <c>` (additive) → promote to `apply+ship` → branch-prep → ship-plan → ship-code |
| tasks all `[x]`, no `feat/<c>` branch, evidence + sync done | `archive-only` | `openspec archive <c> -y --skip-specs --no-validate` |
| already ARCHIVED, OR active but no tasks.md | `skip` | logged; never halts |

**Sorted** by cNNNN ordinal after expanding `c0014` into `c0014a, c0014b,
c0014c`. The numeric ordering respects the dependency graph (proposals were
authored in the order the deps required).

**No confirmation gate.** The slash command launches the real run immediately.
`--dry-run` is an opt-in plan-only invocation (Discover + Plan, writes
`.ship-all-progress.json`, no commits).

**Halt semantics.** Halt on first failure with `{ change, failureStage,
failureLog, mergeSha, archivePath, resumeFrom, summary }`. The merge (if it
happened) is on local `main` — the user is responsible for `git reset` if
they want to undo. **Never** rolls back.

**Resume.** `openspec/changes/.ship-all-progress.json` is the durable state.
Re-running with `--from <cNNNN>` picks up where the previous run stopped;
already-shipped entries are skipped. `--only <c1,c2,...>` is a comma-separated
whitelist (after sorting). `--retry-failed` (planned) re-tries failed entries.

**Flags.**
`--from <cNNNN>`, `--only <list>`, `--dry-run`, `--skip-spec` (default true in
batch — skips the `spec-change` quality pass), `--bump {patch|minor|major}`,
`--push-main`, `--no-archive`, `--merge-strategy {squash|no-ff|ff-only}`,
`--reserve-tokens <n>`, `--max-repairs <n>`, `--force`.

**Skill source of truth.** `.claude/skills/openspec-ship-all/SKILL.md`
documents the decision matrix and halt protocol.

## Practice layer (SDD + TDD skills)

The OpenSpec lifecycle is the spine; the *how* — spec discipline, Red-Green-Refactor,
review, simplification, git/CI hygiene — lives in a set of engineering-practice
skills under `.claude/skills/` (adapted from `addyosmani/agent-skills`). See
[engineering-skills.md](engineering-skills.md) for the full lifecycle map (spec →
design → test → implement → verify → ship) and the evidence convention.

## Handy CLI commands

```bash
openspec list --specs          # list capabilities and requirement counts
openspec show <capability>     # view a spec
openspec validate --specs      # validate all main specs
openspec list                  # list active changes
openspec validate --changes    # validate active changes
```

> Telemetry: OpenSpec collects anonymous usage stats. Opt out with
> `OPENSPEC_TELEMETRY=0` if desired.
