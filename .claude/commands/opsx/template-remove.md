---
name: "OPSX: Template Remove"
description: Delete a task-planning template (openspec/templates/<name>/)
category: Workflow
tags: [workflow, templates, planning]
---

Delete a task-planning **template** — remove its `<templatesDir>/<name>/` folder.
Use when a template is obsolete or no longer matches how the project plans this work.

**Input**:
- `--name <kebab>` — the template to remove, **required**.

**Steps**

1. **Confirm target.** Show the template's `name` + `description` (run
   `node .claude/workflows/lib/templates.js show <name>` to fetch it). If it does not
   exist, say so and stop.

2. **Approval gate.** Use **AskUserQuestion**: "Delete template '<name>'?" with options
   *Delete it*, *Cancel*. Deletion is irreversible (it removes the folder) — do not
   proceed without an explicit choice.

3. **Launch the Workflow:**
   ```
   Workflow({ name: 'template', args: { action: 'remove', name: '<kebab>' } })
   ```
   It runs `node .claude/workflows/lib/templates.js remove <name>`.

4. **Relay the result.** Confirm the template was removed.

**Guardrails**
- Never delete without the approval gate in step 2.
- Removing a template does not touch any change or its tasks.md — it only deletes the
  reusable planning guide.
