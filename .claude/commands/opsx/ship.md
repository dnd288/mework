---
name: "OPSX: Ship"
description: Orchestrate the full ship of an approved OpenSpec change — local merge (no gh) or remote PR — both test-first to an opened PR or locally-merged-and-archived main
category: Workflow
tags: [workflow, automation, pr, tdd, local, experimental]
---

Carry an **approved** OpenSpec change from spec to **shipped** in one go, by
orchestrating the two ship workflows: **ship-plan** (write a `.handoff/<change>/`
grouping the change into a few test-first units) and **ship-code** (execute it
unit-by-unit — Red→Green→one commit per unit — then verify, review, evidence,
sync, and either open a PR or merge locally + archive + open PR).

**Two paths** are offered at the top:

- **Local merge (no gh, no remote push)** — recommended for solo/AI-driven
  ships. Branches `feat/<change>` → per-task commits → Local review → squash-
  merge into `main` → post-merge verify → sync delta specs → archive the change
  → optional semver tag → cleanup. Defaults to fully local
  (`noPushMain=true`); opt in to `git push origin main` with `--push-main`.
- **Remote PR (gh pr create)** — original path. Ends at PR opened; a human
  merges, then `/opsx:archive` finalizes.

**Input**: Optionally a change name (e.g., `/opsx:ship c0006-…`). `--dry-run`
on the remote path makes the per-task commits locally but skips push + PR;
on the local path, refuses merge and stops at Verify (the branch + per-task
commits are still produced).

**Steps**

0. **Path selection.** AskUserQuestion:
   - "Local merge (no gh, no remote push)" — recommended
   - "Remote PR (gh pr create)" — original flow

   **On Local**, AskUserQuestion follow-ups:
   - Merge strategy: `squash` (default) / `--no-ff` / `ff-only`
   - Bump: none (default) / patch / minor / major
   - `noPushMain`: stay fully local (default) / also push `main` to origin
   - Archive: archive after merge (default) / skip archive
   - Review: run local review (default) / skip via `--no-review`

   **On Remote**, the legacy behavior is preserved unchanged.

1. **Select the change** (infer from context / `openspec list --json` +
   AskUserQuestion). Announce "Shipping change: <name> via <path> path".

2. **Plan.** Launch
   `Workflow({ name: 'ship-plan', args: { change, date, local: <true|false> } })`.
   `localOnly` flows through to `plan.json` so `ship-code` picks it up.

3. **Review gate.** Show the handoff: the proposal's what/why, the per-pair
   breakdown, and (for Local) the merge strategy + bump + push + archive
   decisions. Use **AskUserQuestion**: "Handoff looks right — run ship-code
   now?" with options *Ship it*, *Dry run (commits, no push/PR on remote;
   no merge on local)*, *Edit handoff first*, *Cancel*.

4. **Execute.** Launch
   `Workflow({ name: 'ship-code', args: { change, date, dryRun,
     local: <true|false>, base: 'main', mergeStrategy, bump,
     noPushMain, archive, skipReview } })`.

   **Remote path** branches → runs each pair Red→Green→one commit → verify →
   evidence → sync → changelog → push + `gh pr create`.

   **Local path** branches → runs each pair Red→Green→one commit → verify →
   Local review (code-review-and-quality + security-and-hardening, gated on
   `--no-review`) → pre-merge evidence → `git switch main && git merge --<strategy>
   feat/<change>` (conventional commit, signed off, never `git add -A`,
   never auto-resolves conflicts) → re-runs verify on `main` post-merge →
   sync delta specs → archives `openspec/changes/<change>/` →
   `openspec/changes/archive/<date>-<change>/` → optional semver tag →
   chore commit (evidence + sync + archive + changelog + post-merge.md) →
   `git branch -D feat/<change>` → optional `git push origin main` (when
   `--push-main`).

5. **Relay the result.**

   **Remote**: branch, per-task commits, gates + coverage, evidence dir, PR URL.
   Remind the user that **archive happens after merge** (`/opsx:archive`).

   **Local**: mergeSha + baseSha, pre-merge gates + coverage, review verdict +
   findings, post-merge gates + coverage, sync state, archive path, tag (if
   any), choreSha, pushed status, evidence dir (and the new
   `evidence/post-merge.md` inside it).

**Guardrails**
- Never run ship-code without the review gate in step 3.
- Test-first by default; doc-only changes auto-skip Red per pair (recorded, never
  silent).
- The local path **never** calls `gh`. The remote path **always** ends at PR
  opened — never merges.
- Pass `dryRun` whenever testing the pipeline.