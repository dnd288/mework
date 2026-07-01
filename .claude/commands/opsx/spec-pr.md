---
name: "OPSX: Spec PR"
description: Open the spec-only PR for an APPROVED change — sync delta→canonical and gate the contract before code
category: Workflow
tags: [workflow, automation, spec, review, pr, experimental]
---

Open the **spec PR** for an OpenSpec change — stage 2 of the platform workflow
(**spec review & merge**). The change's 6-axis review (`/opsx:spec`) must already have
**APPROVED**. This command merges the change's delta specs into the canonical
`openspec/specs/` and opens a **code-free PR** so a human reviews & merges the *contract*
before any implementation. After it merges, run `/opsx:ship` to implement.

**Input**: Optionally a change name (e.g. `/opsx:spec-pr c0006-…`). If omitted, infer from
context / `openspec list --json`. `--worktree` runs sync + commit inside an isolated git
worktree, then push + PR in the main checkout so the main checkout stays on the base branch.

**Steps**

1. **Select the change** (announce "Opening spec PR for: <name>").

2. **Ask about worktree.** AskUserQuestion: "Where should the spec PR work run?"
   - "Main checkout (default)" — branch and commit in the current working tree
   - "Isolated worktree (`--worktree`)" — branch and commit in a worktree, then push
     from the main checkout. Main checkout stays on the base branch.

3. **Confirm it's ready.** It must have `review/REVIEW.md` with **Verdict: APPROVE** (from
   `/opsx:spec`) and pass `openspec validate "<name>" --strict`. If not, tell the user to
   run `/opsx:spec <name>` first and stop.

4. **Launch the Workflow** (today's date from context, `YYYY-MM-DD`):
   ```
   Workflow({ name: 'spec-pr', args: { change: '<name>', date: '<YYYY-MM-DD>', dryRun: <bool?>, base: '<base?>', worktree: <bool?> } })
   ```
   It creates `spec/<name>`, syncs the delta specs into canonical `openspec/specs/`,
   commits **only** the spec artifacts (proposal + delta + design + tasks + synced
   canonical specs — **no code**), pushes, and opens a `spec(<name>): …` PR with `gh`,
   then **stops** (no auto-merge). When `--worktree` is set, the sync + commit runs in an
   isolated git worktree; push + PR runs in the main checkout.

5. **Relay the result.** Report the branch and PR URL. Tell the user: a human reviews &
   merges the spec PR — when it lands, the contract is on `main`; then run
   `/opsx:ship <name>` to implement against it.

**Guardrails**
- Requires an APPROVED `/opsx:spec` review — never opens a spec PR on an unreviewed draft.
- Spec only: the commit stages **only** `openspec/changes/<name>/` + `openspec/specs/` — if
  any source/app/package file would be staged, it stops.
- Opens the PR and **stops** — never merges. A human merges.
- The change stays **ACTIVE** for implementation; it is archived later (after the code PR).
- `--dry-run` commits on `spec/<name>` without pushing or opening the PR.
- `--worktree` runs sync + commit in an isolated worktree; push + PR run in main checkout.
- All git ops happen inside the `platform/` submodule; the superproject gitlink bump is manual.
