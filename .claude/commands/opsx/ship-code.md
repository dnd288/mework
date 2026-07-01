---
name: "OPSX: Ship Code"
description: Execute a ship-plan handoff unit-by-unit — Red→Green→one commit per unit, then verify/review/evidence/sync; remote → PR, local → merge + open PR
category: Workflow
tags: [workflow, automation, tdd, pr, experimental]
---

Execute the `.handoff/<change>/` handoff produced by `/opsx:ship-plan`, **unit by
unit and test-first**. For each unit (a few per change, each may span several files)
it writes the failing test(s) (Red), implements the minimal code to pass (Green), and
makes **one commit for the unit**. After all units it runs the full verify gates and
review. The **remote** path then writes evidence, syncs delta specs, updates the
changelog, and opens a PR (**stops at PR opened** — a human merges). The **local**
path (`--local`) reviews → merges `feat/<change>` into `main` locally → archives →
and with `--openPr` pushes the branch + opens a PR for the record. This is the second
half of `/opsx:ship`.

**Input**: Optionally a change name (e.g., `/opsx:ship-code c0006-…`). `--base <branch>`
sets the branch the change is built on and the PR targets (default `main`). `--dry-run`
makes the per-unit commits locally but skips push/PR/merge. `--only <unit>` runs a
single unit (e.g. `--only 02`); `--retry-blocked` re-runs blocked units. `--local`,
`--openPr` select the local-merge + PR path (merge into `<base>` locally, then optionally
open a PR for the record).
`--worktree` runs implementation inside an isolated git worktree so the main checkout
stays on `<base>`. For the remote path (`--worktree` without `--local`), it stops after
the chore commit (no push, no PR) for human local verification. For the local path
(`--local --worktree` or `--local-worktree`), it runs implementation in the worktree then
automatically does the local merge/archive/cleanup in the main checkout.

On the **remote** paths, a **Base sync** phase runs before preflight: `git fetch origin
<base>`, switch to `<base>`, `git merge --ff-only origin/<base>` so the change is built
on the latest base. It needs a clean tree and a fast-forwardable base — it never
auto-stashes or force-resets; either condition stops the ship with an actionable reason.
(`--local` skips this — it ships from an already-checked-out `feat/<change>` and merges
into its base in the Merge phase.)

**Steps**

1. **Select the change** and confirm a handoff exists: check
   `.handoff/<name>/plan.json`. If missing, tell the user to run
   `/opsx:ship-plan <name>` first and stop.

2. **Approval gate.** Summarize the handoff (units + their test/code deliverables)
   and a clean-tree note. Use **AskUserQuestion**: "Run ship-code on this handoff?"
   with options *Ship it*, *Dry run (commits, no push/PR)*, *Cancel*. Don't proceed
   without an explicit choice (unless the user already said to).

3. **Ask about worktree (for local path).** If `--local` is set but neither `--worktree`
   nor `--local-worktree` was passed, ask: "Where should the implementation run?"
   - "Main checkout (default)" — implement directly on the current branch
   - "Isolated worktree (`--local-worktree`)" — implement in a worktree, then
     merge/archive/cleanup in the main checkout

4. **Launch the Workflow** (date from context):
   ```
   Workflow({ name: 'ship-code', args: { change: '<name>', date: '<YYYY-MM-DD>', base: '<branch?>', dryRun: <bool>, only: '<unit?>', retryBlocked: <bool?>, local: <bool?>, openPr: <bool?>, worktree: <bool?>, localWorktree: <bool?> } })
   ```
   Phases: **Base sync** (fetch origin + switch to `<base>` + `merge --ff-only
   origin/<base>`) → **Preflight** (toolchain check, validate, clean tree, branch
   `feat/<name>`, load handoff units) → **Implement in worktree** (when `--worktree` or
   `--local-worktree`: all phases in isolated git worktree) / **Implement** (per unit:
   Red → Green → one commit) → **Verify** (resolver-selected per-toolchain gates —
   `uv`/`go`/`pnpm` + `ci-free-gates.sh` + coverage + `openspec validate`, repair loop)
   → **Review** → **Evidence** → **Sync** → (local: **Merge** → **Archive** → **Open PR**)
   / (remote: **Changelog** + chore → **PR**).

5. **Relay the result.** Report the branch, the **per-unit commits** (one per unit,
   each red+green), the chore commit, verify gates + coverage, evidence dir, and the
   **PR URL** (and on `--local`, the mergeSha + archivePath). On dry run: the local
   commits to inspect (`git log --stat`). On a blocked unit / failed verify / budget
   stop: surface the `stage` + `reason`; completed commits are on the branch.

**Guardrails**
- Never launch without the gate in step 2; never without a handoff.
- Each unit = exactly one commit (its failing test(s) + the implementation).
- Remote path does not merge or archive (after merge, run `/opsx:archive <name>`);
  the local path merges + archives itself, and with `--openPr` opens a PR for review.
- `--worktree` (without `--local`) stops after implementation — push + PR via `/opsx:ship-pr`.
  `--local --worktree` / `--local-worktree` does implement + merge + archive automatically.
- The worktree is cleaned up automatically after the agent returns (success or failure).
- The Preflight toolchain check resolves which toolchains the change touches and stops early
  if a needed tool is missing (`go` < 1.24, or `uv`/`pnpm`/`openspec`/`gh`/`node`).
- A merged **spec PR** (`/opsx:spec-pr`) is a prerequisite — Preflight asserts the canonical
  contract is on the base branch, and the Sync phase only **reconciles** (drift → stop).
