---
name: "OPSX: Ship All"
description: Auto-discover every ACTIVE OpenSpec change and ship the full project — apply → ship → archive — locally, automatically, with halt-on-failure
category: Workflow
tags: [workflow, automation, batch, ship, local, experimental]
---

Ship **every ACTIVE OpenSpec change** through the local pipeline in one go,
**fully automatically — no confirmation, no per-change prompts**. The orchestrator
auto-decides per change whether to apply+ship, spec+ship, ship-only, repair+ship,
or archive-only. Each change keeps the full workflow — branch → a **few test-first
units** (Red→Green→one commit per unit, not one per task) → verify → review — then
**merges into `main` locally** (so the next dependency-ordered change builds on it)
**and opens a PR** for the record/human review. Result: the project on `main` + one
PR per change. Halts on first failure with full progress.

**Input**: Optional `--from <cNNNN>` (start from this ordinal), `--only <list>`
(comma-separated whitelist), `--dry-run` (plan-only), `--skip-apply` (treat all
changes as already-implemented), `--skip-spec` (default true in batch — skip
the 6-critic spec quality pass), `--bump {patch|minor|major}`, `--push-main`,
`--no-archive`, `--merge-strategy {squash|no-ff|ff-only}`, `--force`.

Defaults: squash merge, noPushMain=true, archive=true, skipSpec=true (faster),
skipApply=false. Per-change evidence is always written.

**Steps**

1. **Launch the workflow directly — no confirmation gate.** The command runs the
   full pipeline immediately:
   ```
   Workflow({ name: 'ship-all', args: {
     date: <today YYYY-MM-DD>, fromChange, onlyChange, skipApply, skipSpec,
     mergeStrategy, bump, noPushMain, archive, reserveTokens, maxRepairs, force
   }})
   ```
   The orchestrator's Discover + Plan phases write the queue to
   `openspec/changes/.ship-all-progress.json`, then it ships every change in
   cNNNN order without stopping to ask. Pass `--dry-run` only if the user
   explicitly wants a plan-only run (Discover + Plan, no commits) — otherwise
   default to the real run.

2. **Surface progress.** As each change ships, show the result: change name,
   mode, evidence dir, mergeSha, archivePath, tag, **PR url**. On halt, show the
   failing change + reason + last commit + suggested fix.

3. **Relay the final report.** Per-change summary table (change | mode |
   commits | mergeSha | archivePath | tag | **prUrl**) + total stats + resume
   instructions. Always includes the `resumeFrom` change so a halted run can be
   picked up with `--from`.

**Guardrails**
- **Fully automatic** — never asks the user, per change or overall.
- Halt on first failure. **Never** roll back merged changes. The merge is local;
  if the user wants to undo it: `git reset --hard <pre-merge-sha>`.
- Per-change evidence is always written, even on failure.
- The orchestrator starts each change from a clean `main` and creates
  `feat/<change>` itself; a globally dirty tree (tracked files outside `.claude/`)
  trips the per-change branch-prep, which halts cleanly with a "commit/stash
  first" reason.
- `--dry-run` is an opt-in plan-only invocation — it surfaces the queue without
  committing anything.
- Skill `.claude/skills/openspec-ship-all/SKILL.md` is the source of truth
  for the per-change decision matrix.
- Resume: re-running with `--from <cNNNN>` picks up where the previous run
  stopped. Already-shipped entries are skipped (status check via the per-change
  `openspec status` — entries with `feat/<c>` deleted and `archive/<c>` present
  are considered done).