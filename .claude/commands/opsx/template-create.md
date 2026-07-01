---
name: "OPSX: Template Create"
description: Author a new, skill-like task-planning template (a reusable planning guide) under openspec/templates/
category: Workflow
tags: [workflow, templates, planning, authoring]
---

Author a new task-planning **template** — a skill-like, reusable planning guide for a
recurring kind of work. It lands as `<templatesDir>/<name>/TEMPLATE.md` (default
`openspec/templates/<name>/`) with `name` + `description` frontmatter and a body that
tells a planner HOW to break this kind of change into tasks for THIS project. Templates
are optional; create one only when a flow recurs often enough to be worth capturing.
This is the reverse of consulting a template during planning.

**Input** — a name plus one content source:
- `--name <kebab>` — the template id (kebab-case), **required**.
- `--description "<text>"` — one line: *when this template applies*.
- `--prompt "<text>"` — describe how this kind of work should be planned, **or**
- `--from-change <name>` — generalize how an existing change was planned into a reusable guide.

**Steps**

1. **Draft.** The workflow's Resolve phase reads the prompt / change and drafts the
   guide body + description. Preview it.

2. **Approval gate.** Show the drafted `name`, `description`, and guide body, and where
   it will be written. Use **AskUserQuestion**: "Create this template?" with options
   *Create it*, *Edit first*, *Cancel*. Don't write without an explicit choice.

3. **Launch the Workflow:**
   ```
   Workflow({ name: 'template', args: { action: 'create', name: '<kebab>', description: '<?>', prompt: '<?>', fromChange: '<?>' } })
   ```
   Resolve (draft the guide) → Apply (`templates.js create <name> --description "<d>"`
   with the body piped via stdin).

4. **Relay the result.** Report the `name`, `file` path, and that it will be offered
   during planning when it matches a change.

**Guardrails**
- Never write a template without the approval gate in step 2.
- A guide is a **planning playbook**, not the deliverable content — it says how to plan
  tasks, and should reference existing project standards rather than restate them.
- Names are kebab-case; `create` refuses to clobber an existing template (use
  `/opsx:template-update`).
