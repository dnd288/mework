---
name: "OPSX: Ship PR"
description: Push an implemented feature branch and open/create its code PR — after local verification of a --worktree ship
category: Workflow
tags: [workflow, automation, pr, experimental]
---

Push the `feat/<change>` branch and create (or update) the code PR — in one step.
The change is **not archived here**: archiving happens at merge time via
`/opsx:merge-pr` (which commits the archive on the branch before merging, so it
still lands on `main` with the merge). Keeping the change active through PR review
means `/opsx:address-review` can keep operating on the live change directory.

> **Prerequisite:** The change must have been implemented already via
> `/opsx:ship --worktree <change>`. Test locally, then run this.

**Input**: A change name (e.g., `/opsx:ship-pr c0006-…`).

**Steps**

1. **Select the change** (infer from context / `openspec list --json` /
   AskUserQuestion). Announce "Opening code PR for: <name>".

2. **Verify the branch exists:**
   ```bash
   git rev-parse --verify feat/<change>
   ```
   If it doesn't exist, stop — tell the user to run `/opsx:ship --worktree <change>`
   first.

3. **Push the branch** (all implementation + evidence commits — do NOT archive here):
   ```bash
   git push -u origin feat/<change>
   ```

4. **Create or update the PR.** Reuse if one already exists:
   ```bash
   gh pr view feat/<change> --json url,state 2>/dev/null
   ```
   If open → reuse its URL. Otherwise create:
   ```bash
   gh pr create --base main --head feat/<change> \
     --title "feat: <title from proposal>" \
     --body "<summary from proposal + AI review findings + evidence>"
   ```

5. **Relay the result.** Report the PR URL. Tell the user: the change stays active
   through review; when the PR is merged via `/opsx:merge-pr`, the archive is
   committed on the branch and lands on `main` with the merge.

**Guardrails**
- Requires a `feat/<change>` branch with commits — never creates a PR from nothing.
- Does not implement code, does not archive, does not merge — just pushes + creates the PR.
- Archiving is owned by `/opsx:merge-pr` (or a standalone `/opsx:archive` after merge),
  never by ship-pr — the change must stay active for `/opsx:address-review`.
- Handles the case where the PR already exists (push updates it, reuses URL).
