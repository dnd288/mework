---
name: "OPSX: Propose"
description: Scaffold a new OpenSpec change from a free-text prompt — compute the next cNNNN-<slug>, create the change, and seed proposal.md. The github-agnostic front door to the pipeline.
category: Workflow
tags: [workflow, openspec, propose, scaffold]
---

Start a new change from a description of what to build. `/opsx:propose`
**only scaffolds** — it computes the next `cNNNN-<slug>`, runs `openspec new change`,
and drafts `proposal.md` grounded in your prompt. It does not review, validate, or
touch any task source; that separation is deliberate.

Flow: `/opsx:propose <what>` → `/opsx:spec` → `/opsx:spec-pr` → `/opsx:ship-*`.

**The prompt can be either**:
- **Inline text** — a free-text description of the change
- **A file pointer** — "read the task from `<path>`" where an adapter has written
  the task content to a local file

> To start from a GitHub issue and link it, use the **task-github** extension:
> `/opsx:propose-gh <issue>` instead.

**Options**
- `--slug <kebab>` — pin the kebab slug (otherwise derived from the prompt)
- `--worktree` — create a **persistent spec worktree** (`../<project>-spec-<cNNNN-<slug>/`)
  on a `spec/<cNNNN-<slug>` branch off `main`. All scaffolding happens inside the
  worktree, so the main checkout stays on the base branch and multiple specs can be
  worked on simultaneously.
- `--base <branch>` — base branch for the spec worktree (default: main)

**Steps**

1. **Confirm scope.** Restate the change in one line and the `cNNNN-<slug>` it will create.
2. **Launch the Workflow** (date from context):
   ```
   Workflow({ name: 'propose', args: { prompt: '<what>', slug: '<slug|undefined>', date: '<YYYY-MM-DD>', worktree: true|false } })
   ```
   With `--worktree`, it adds a **Worktree** phase before **Scaffold**: creates the git
   worktree, scaffolds the change inside it, then returns the worktree path.
3. **Relay the result.** Report the created `change`, `proposalPath`, and next step.

**Guardrails**
- Faithful seeding only: `proposal.md` must reflect the prompt; do not invent scope.
- Do not review or validate here — that is `/opsx:spec`'s job.
- One change per concern — prefer splitting an over-broad request over packing it.
