---
name: "OPSX: Template List"
description: List the optional, skill-like task-planning templates (openspec/templates/) — read-only
category: Workflow
tags: [workflow, templates, planning]
---

List the project's task-planning **templates** — the optional, skill-like planning
guides under `templatesDir` (default `openspec/templates/`). Each template is a
`<name>/TEMPLATE.md` with `name` + `description` frontmatter and a body that is the
planning guide. Templates are optional: during planning the agent consults this
catalog and applies one only if it matches the change; **most of the time there is no
template and planning proceeds normally.** This command is read-only.

**Input**: none.

**Steps**

1. **Launch the Workflow:**
   ```
   Workflow({ name: 'template', args: { action: 'list' } })
   ```
   It runs `node .claude/workflows/lib/templates.js list --json`.

2. **Relay the catalog.** Show each template's `name` + `description`. If the catalog
   is empty, say so plainly — that is the normal case — and point to
   `/opsx:template-create` for when a flow recurs often enough to capture.

**Guardrails**
- Read-only — never create, edit, or delete a template.
