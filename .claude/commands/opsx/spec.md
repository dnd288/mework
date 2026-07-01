---
name: "OPSX: Spec"
description: Author and quality-assure an OpenSpec change spec — cross-validate across the 6 review axes, then revise until clean
category: Workflow
tags: [workflow, automation, spec, review]
---

Take an OpenSpec change's spec from draft to **clean, minimal, testable, and
complete** before any code is written. Fans out one read-only critic per spec-review
axis in parallel, then runs a revise loop that fixes Blocker/Required findings and
re-runs `openspec validate --strict` until clean.

Use this after `/opsx:propose` and before implementation. The quality bar is the
`spec-review-and-quality` skill (six axes: Structure/validity · Clarity/KISS ·
Testability · Minimality/YAGNI · Consistency/DRY · Completeness).

**Input**: A change name (`/opsx:spec c0002-message-bus`). To draft a brand-new
change, pass `--new <slug>`. Pass `--dry-run` for review-only (no edits).

**Options**
- `--new <slug>` — draft a new change (scaffold it, then review)
- `--dry-run` — review only, no edits
- `--max-revisions <N>` — max revise iterations (default: 2)
- `--worktree` — work from the persistent spec worktree at
  `../<project>-spec-<change>/`. If the worktree doesn't exist, it's created.
  All spec authoring happens inside the worktree; the main checkout stays clean.

**Steps**

1. **Select the target.** Either an existing change name, or — for a new spec — a
   kebab slug to draft. Announce: "Reviewing spec: <name>" and how to override.

2. **Gate (AskUserQuestion).** Confirm intent:
   - For an existing change, run a one-line summary (title, delta specs, task count).
   - Ask: "Review and revise this spec?" with *Review & revise*, *Review only (dry run)*,
     *Cancel*. Do not proceed without a choice.

3. **Launch the Workflow** (date from context, use `currentDate`):
   ```
   Workflow({ name: 'spec-change', args: { change: '<name>', slug: '<slug|undefined>', date: '<YYYY-MM-DD>', dryRun: <true|false>, maxRevisions: <2>, worktree: <true|false> } })
   ```
   With `--worktree`, it adds a **Worktree** phase before **Preflight** that creates
   or detects the persistent spec worktree and runs all subsequent phases inside it.

4. **Relay the result.** Report verdict, revisions, validate status, review report path,
   and next step. With `--worktree`, report the worktree path.

**Guardrails**
- Never launch without the gate in step 2.
- `dryRun: true` makes **no edits**.
- Scaffolding a new change requires `--new <slug>` and a non-dry run.
- With `--worktree`, the change's artifacts live in the worktree; the main checkout
  stays on the base branch.
