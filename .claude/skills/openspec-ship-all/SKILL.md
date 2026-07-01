---
name: openspec-ship-all
description: Auto-discover every ACTIVE OpenSpec change and ship the full project — apply → ship → archive — locally, automatically, with halt-on-failure and idempotent resume. Use when the user wants `/opsx:ship-all`, "ship everything", "batch ship", "ship the full project", or "run the full pipeline".
license: MIT
compatibility: Requires openspec CLI + ship-plan.js + ship-code.js + openspec-apply-change skill.
metadata:
  author: mework
  version: "1.0"
  generatedBy: "ship-all orchestrator"
---

# Ship All — the per-change decision matrix

This is the source of truth for how the `ship-all` orchestrator decides what to
do with each ACTIVE OpenSpec change. The orchestrator runs **fully automatically**
— it never asks the user (per change or overall); every decision comes from this
matrix.

## Branch + execution model (applies to every ship mode)

The orchestrator owns branch creation; `ship-code` owns the implementation. For
every change that ships, the orchestrator first runs **branch-prep** — from a clean
`main` it creates (or, on resume, reuses) `feat/<change>`. It then invokes the
nested workflows via the lowercase `workflow()` helper (NOT by spawning an agent
that calls `Workflow()` — workflow-spawned subagents have no `Workflow()` tool):

1. `workflow('ship-plan', { change, date, local: true })` — groups the change's
   **open tasks** into a **few test-first units** (aim 1-4; each unit may span
   several files) under `.handoff/<change>/` (gitignored). NOT one unit per
   tasks.md line.
2. `workflow('ship-code', { change, date, local: true, openPr: true, base: 'main',
   bump, noPushMain, archive, mergeStrategy, reserveTokens, maxRepairs })` — runs
   each unit **test-first** (Red→Green→one commit per unit), verifies, **reviews**
   (code-review-and-quality + security), then **merges `feat/<change>` into `main`
   locally** (so the next dependency-ordered change builds on it), syncs delta
   specs, archives, optionally tags, and **pushes the branch + opens a PR** for the
   record/human review.

There is **no standalone `/opsx:apply`** step — `ship-code` does the implementation
test-first per unit. (A bulk apply would write uncommitted code and trip
`ship-code`'s clean-tree preflight.) Result of a full run: the project on `main`
plus one PR per change.

## Per-change decision

For each ACTIVE change, the orchestrator runs `openspec status --change <c> --json`
and `openspec list --json`, then classifies the change by **mode**:

| Mode | Trigger | Steps run by orchestrator |
|---|---|---|
| `apply+ship` | Active, has proposal + design + tasks + specs + `.openspec.yaml`, **0 tasks done** | branch-prep (`feat/<c>` from `main`) → `ship-plan` (few units) → `ship-code --local --openPr` (implements each unit test-first, one commit per unit, then review → local merge → archive → open PR) |
| `spec+ship` | Active, all artifacts present, **all tasks `[x]`**, no evidence dir | branch-prep → (`workflow('spec-change', { change, maxRevisions: 1 })` when not `--skip-spec`) → `ship-plan` → `ship-code --local --openPr` |
| `ship-only` | Active, all tasks `[x]`, evidence dir exists, delta spec already in `openspec/specs/` | branch-prep → `ship-plan` (0 units) → `ship-code --local --openPr` (verifies existing code, review, local merge, archive, open PR) |
| `repair+ship` | Active, **missing `.openspec.yaml`** (scaffolding-only) | 1. `openspec new change <c>` (additive — does NOT touch existing proposal/design/tasks/specs) → 2. promote to `apply+ship` → 3. branch-prep → ship-plan → ship-code |
| `archive-only` | Active, all tasks `[x]`, no `feat/<c>` branch, evidence dir + delta-spec sync complete, no code work expected | 1. `openspec archive <c> -y --skip-specs --no-validate` |
| `skip` | Already ARCHIVED, OR active but no tasks.md (incomplete proposal) | Logged in `.ship-all-progress.json` → `progress.skipped`; never halts the run |

## Cues for the orchestrator agent

1. **Always run `openspec list --json` + per-change `openspec status --change <c> --json` BEFORE deciding modes.** No guessing from the directory listing alone — `cNNNN` filename can be a complete or incomplete proposal.
2. **Sort by cNNNN ordinal** (lexicographic) AFTER expanding `c0014` into `c0014a, c0014b, c0014c`. The numeric ordering respects the dependency graph because each change's proposal was authored in the order the deps required.
3. **Apply `--from <cNNNN>` and `--only <list>` filters** AFTER sorting, so the queue is consistent.
4. **`--dry-run`** runs only Phases 1-2 (Discover + Plan), writes `.ship-all-progress.json`, returns the planned queue. Never commits.
5. **`--skip-spec`** skips the `spec-change` quality pass for `spec+ship` entries (they go straight to `ship-plan` → `ship-code`). Default in batch mode (the 6-critic spec quality pass is too expensive to run for every change in a batch).
6. **Budget reserve**: the orchestrator passes `reserveTokens` to every nested `workflow()` call so a budget hit in ship-code halts the entire run cleanly (not mid-pair).
7. **Nesting**: invoke `ship-plan`/`ship-code`/`spec-change` with the lowercase `workflow()` helper (one level of nesting). Never spawn an `agent()` that tries to call `Workflow()` — those subagents lack the tool.

## Halt semantics

The orchestrator **halts on first failure** and returns:

```json
{
  "stage": "apply+ship" | "spec+ship" | "ship-only" | "repair+ship" | "archive-only",
  "ok": false,
  "change": "c0008-object-storage",
  "reason": "...",
  "mergeSha": "...",          // present if merge already happened
  "archivePath": "...",       // present if archive already moved
  "resumeFrom": "c0009-sessions",  // next change to process on resume
  "progressLog": "openspec/changes/.ship-all-progress.json",
  "nextStep": "Fix the failing change locally, then re-run /opsx:ship-all --from <resumeFrom>"
}
```

**Never** rolls back a merged change. The user must `git reset --hard <pre-merge-sha>` themselves if they want to undo.

## Resume protocol

The `.ship-all-progress.json` is the durable state. On re-run:

1. Read the progress file. For each entry with `status=shipped`, **skip** (it's done).
2. For each entry with `status=pending`, **process** (per mode).
3. For each entry with `status=failed`, **skip with warning** (the user must explicitly clear it via `--retry-failed` or fix the change and re-add).

Use `--from <cNNNN>` to start at a specific ordinal (overrides the resume).

## Output

The orchestrator's final report is a per-change summary table + a single
`.ship-all-progress.json` updated atomically (`fs.writeFileSync` with
`JSON.stringify(progress, null, 2)`) after each change so a halt never loses
state.