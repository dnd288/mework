---
name: "OPSX: Ship"
description: Ship an approved OpenSpec change — plan + implement + open a PR (remote) or merge locally
category: Workflow
tags: [workflow, ship, pr, tdd]
---

Implement an OpenSpec change and open its code PR (remote, default) or merge
directly into the base branch (local). Runs **ship-plan** (group tasks into
test-first units) then **ship-code** (Red→Green→commit per unit → verify →
review → evidence → PR/merge).

**One prerequisite:** the spec must be reviewed & merged. Run `/opsx:spec` →
`/opsx:spec-pr` first if not done.

**Input:** Change name (e.g., `/opsx:ship restructure-addon-preview`). Flags:
- `--local` — merge directly into the base branch (no PR, no gh). Use for solo/trivial changes.
- `--worktree` — run implementation in an isolated git worktree (main checkout stays clean).
- `--base <branch>` — base branch (default: `main`).
- `--dry-run` — commits only; no push, no PR, no merge.

**Steps**

0. **Path.** If `--local` was passed → local merge. Otherwise → remote PR (default).
   `--worktree` adds worktree isolation to either path.

0b. **Base branch.** If `--base` was passed, use it. Otherwise ask once — default `main`.

1. **Select change.** Infer from context or `openspec list --json`. Announce:
   "Shipping `<change>` via `<path>` onto `<base>`" + any flags.

2. **Plan.** Launch `Workflow({ name: 'ship-plan', args: { change, date, local: <true|false> } })`.

3. **Review gate.** Show the handoff breakdown. Ask:
   - **Ship it** (default)
   - **Dry run** (commits only on remote; no merge on local)
   - **Edit handoff first** (stops)
   - **Cancel**

4. **Execute.** Launch `Workflow({ name: 'ship-code', args: { change, date, dryRun, local, worktree, base } })`.

   **Remote path:** Branch from freshened base (`git fetch; merge --ff-only origin/<base>`) →
   per-unit commits → verify → agent review → evidence → reconcile delta specs →
   changelog → push + `gh pr create` with review findings + spec PR link. **Stops at PR opened.**

   **Local path:** Branch → per-unit commits → verify → review (code + security) →
   evidence → `git merge --squash feat/<change>` into base → post-merge verify →
   sync delta specs → archive change → optional tag → cleanup branch. No gh.

   **`--worktree` on either path:** implementation runs in an isolated git worktree.
   Remote-worktree stops after implementation (no push/PR — run `/opsx:ship-pr`).
   Local-worktree auto-merges from worktree then cleans up.

5. **Result.**

   **Remote:** PR URL, branch, per-unit commits, gates + coverage, review verdict.
   Remind: human reviews & merges; `/opsx:address-review` for feedback; archive after merge.

   **Local:** merge SHA, gates + coverage, review findings, archive path.

**Guardrails**
- Never skip the review gate (step 3).
- Test-first by default; doc-only units set `skipRed` explicitly.
- `--dry-run` on remote: commits only, no push/PR.
- `--dry-run` on local: stops at verify, no merge.
