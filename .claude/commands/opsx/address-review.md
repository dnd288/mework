---
name: "OPSX: Address Review"
description: Address human review feedback on a change's code PR — fix, re-verify, push, reply (no merge)
category: Workflow
tags: [workflow, automation, review, pr, feedback, experimental]
---

Address human review feedback on the **code PR** for an OpenSpec change — stage 5 of the
platform workflow (**PR review**). It finds the open `feat/<change>` PR, addresses the
actionable review comments, re-runs the resolver gates on the touched files, pushes, and
replies to/resolves the threads — then **stops** for human re-review. It never merges.

**Input**: Optionally a change name (e.g. `/opsx:address-review c0006-…`). If omitted, infer
from the current branch / context.

**Steps**

1. **Select the change** (announce "Addressing review for: <name>").

2. **Launch the Workflow:**
   ```
   Workflow({ name: 'address-review', args: { change: '<name>', dryRun: <bool?>, base: '<base?>' } })
   ```
   It collects the PR's unresolved review threads via `gh`, makes the minimal fix for each
   actionable one, re-verifies the touched packages, commits + pushes, and replies/resolves.

3. **Relay the result.** Report the PR URL, how many comments were addressed, the fix
   commits, and any threads it intentionally left for the human (e.g. ones that would change
   the merged spec contract — those go back through `/opsx:spec` + `/opsx:spec-pr`, not here).

**Guardrails**
- Never merges or approves the PR — a human re-reviews after the fixes.
- A comment that contradicts the **merged spec contract** is NOT silently applied — it's
  flagged for the human; contract changes require a new spec review + spec PR.
- Re-runs the resolver-selected gates on the touched files before pushing; if they fail,
  nothing is pushed.
- `--dry-run` lists the comments it would address without editing or pushing.
- All git/gh ops happen inside the `platform/` submodule.
