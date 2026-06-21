---
name: "OPSX: Ship Plan"
description: Plan an approved OpenSpec change as a reviewable .handoff/<change>/ — a few test-first units (not one per task)
category: Workflow
tags: [workflow, automation, planning, tdd, experimental]
---

Turn an **approved** OpenSpec change into a reviewable **execution handoff** under
`.handoff/<change>/` — without writing any code. It groups the change's `tasks.md`
into **a few test-first units** (aim 1–4; each may span several files, covering one or
more tasks) — **not** one unit per task. Each unit lists its `testDeliverables` (the
failing tests to write first) and `codeDeliverables` (the production change).
`/opsx:ship-code` later executes them, **one Red→Green commit per unit**. This is the
first half of `/opsx:ship`.

**Input**: Optionally a change name (e.g., `/opsx:ship-plan c0006-…`). If omitted,
infer from context / `openspec list --json` + AskUserQuestion.

**Steps**

1. **Select the change** (announce "Planning change: <name>").

2. **Confirm it's plannable.** Run `openspec status --change "<name>" --json` and
   `openspec validate "<name>"`. Show the open task count from `tasks.md`. If the
   change isn't approved/validated, say so and stop.

3. **Launch the Workflow** (today's date from context, `YYYY-MM-DD`):
   ```
   Workflow({ name: 'ship-plan', args: { change: '<name>', date: '<YYYY-MM-DD>' } })
   ```
   It writes `.handoff/<name>/plan.json`, `README.md`, and one
   `tasks/<NN>-<slug>.md` per unit (a few, not 2×N). `.handoff/` is
   gitignored — it's local execution scaffolding.

4. **Show the handoff for review.** Report the handoff path and the per-unit
   breakdown (each unit → its test deliverables + code deliverables + covered
   tasks). Invite the user to inspect/edit the unit files before running
   `/opsx:ship-code`.

**Guardrails**
- Planning only — no branch, no code, no commits.
- A few units, not one per `tasks.md` line — group along package/capability seams.
- Idempotent: re-running preserves any unit already marked `done`.
- If `tasks.md` has no open tasks, report nothing to plan.
