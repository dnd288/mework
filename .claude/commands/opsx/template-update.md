---
name: "OPSX: Template Update"
description: Revise an existing task-planning template's guide (openspec/templates/<name>/TEMPLATE.md)
category: Workflow
tags: [workflow, templates, planning, authoring]
---

Revise an existing task-planning **template** — update its planning guide and/or
description in place. The template stays at `<templatesDir>/<name>/TEMPLATE.md`; only
its content changes.

**Input**:
- `--name <kebab>` — the template to update, **required**.
- `--prompt "<text>"` — what to change about how this work is planned.
- `--description "<text>"` — (optional) replace the one-line "when this applies".

**Steps**

1. **Load + draft.** The Resolve phase reads the current guide
   (`templates.js show <name>`) and drafts the revision per `--prompt`, preserving what
   still applies. Preview the diff/result.

2. **Approval gate.** Show the revised guide and use **AskUserQuestion**: "Apply this
   update?" with options *Apply it*, *Edit first*, *Cancel*. Don't write without a choice.

3. **Launch the Workflow:**
   ```
   Workflow({ name: 'template', args: { action: 'update', name: '<kebab>', prompt: '<?>', description: '<?>' } })
   ```
   Resolve (load + revise) → Apply (`templates.js create <name> --force` with the new
   body piped via stdin).

4. **Relay the result.** Report the `name` and `file` path.

**Guardrails**
- Never write without the approval gate in step 2.
- Faithful revision: keep the guide a planning playbook; don't turn it into the
  deliverable content.
- Fails clearly if the template does not exist — use `/opsx:template-create` first.
