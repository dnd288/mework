---
name: openspec-propose
description: Creates a new OpenSpec change with the standard artifact structure — proposal.md, design.md, tasks.md, ui.md, and delta specs. Detects when a change has a user-facing surface and creates ui.md. Includes design language discovery guidance.
tags: []
---

# Openspec Propose

Creates a new OpenSpec change with the standard artifact structure. Run:

```
node .claude/workflows/lib/openspec.js new change "<name>"
```

This scaffolds `openspec/changes/<name>/` with `proposal.md`, `design.md`, `tasks.md`, and `specs/`.

Then draft:
- **proposal.md** — what, why, scope, assumptions
- **Delta specs** under `specs/<capability>/spec.md` using `ADDED/MODIFIED/REMOVED/RENAMED`
- **design.md** — architectural decisions and rationale
- **ui.md** — UI/visual design (optional; create only for changes with a user-facing surface)
- **tasks.md** — ordered, verifiable implementation tasks

Name convention: `cNNNN-<kebab-slug>` starting from the next available number.

## Detecting when to create ui.md

If the change has a visible user-facing surface — new screens, component changes,
UX improvements, or UI-adjacent work — create `ui.md` following the `ui-design`
skill (`openspec/changes/<name>/ui.md`). Check the prompt for keywords like
"UI", "page", "screen", "component", "form", "frontend", "portal", "dialog",
"modal", "layout", "navigation", "theme", "responsive", or "mobile" to determine
if UI design is needed.

If the change is purely backend, infrastructure, or spec-only with no UI surface,
skip `ui.md`. State "no UI surface — ui.md not created" in the propose output.

## Design language discovery

Before creating `ui.md`, discover what design documentation the project already has.
Every project is different — some have a comprehensive DESIGN.md, others have design
skills, others have nothing but the code.

1. **Check for existing design documentation:**
   - `DESIGN.md` or `DESIGN_SYSTEM.md` at the project root
   - `docs/design/` directory or `docs/design.md`
   - `.claude/skills/*/SKILL.md` files that reference design, UI, or UX
   - `STYLE_GUIDE.md`, `BRAND.md`, or similar
   - `openspec/patterns/ux-patterns.md` (UX pattern reference, if previously created)

2. **If `openspec/patterns/ux-patterns.md` exists:** Read it. It contains the
   project's UX patterns, abstract business flows, and visual concepts. Reference
   it when creating `ui.md` — the `ui-design` skill expects it.

3. **If it does NOT exist but the project has other design documentation:**
   Read DESIGN.md or equivalent and reference what you find. The `ui-design` skill's
   Step 1 (Discover the Project's Design Language) will guide the process.

4. **If NO design documentation exists at all:** Note this in the `ui.md` "Design
   Language Reference" section. The design will be anchored to patterns discovered
   by reading the codebase directly. Consider running the `ux-pattern-audit` skill
   as a separate change to establish the baseline.

The key insight: **every project already has a design language** — in its DESIGN.md,
its codebase conventions, its component patterns. The `ui.md` must find and reference
it, not invent from scratch.

## Task tagging

When creating `tasks.md`, annotate each task with tags that classify the type of work:

```markdown
## Task [1]: Login form UI  (tags: ui, auth)

## Task [2]: Token validation API  (tags: backend, api, security)

## Task [3]: User table migration  (tags: db, migration)

## Task [4]: Setup CI pipeline  (tags: infra)
```

Tags drive the tag-driven skill routing system. During `ship-plan`, units inherit tags
from the tasks they cover. During `ship-code`, only skills matching the unit's tags are
loaded — no irrelevant guidance for backend units, no missed UI patterns for frontend.

Standard tags (see the `tag-system` skill for the full taxonomy):
- `ui` — components, screens, user-facing surfaces
- `backend` — server logic, APIs, services
- `api` — endpoints, contracts
- `db` — schemas, queries, migrations
- `auth` — authentication, authorization
- `security` — hardening, vulnerability fixes
- `config` — configuration, setup
- `docs` — documentation, ADRs
- `infra` — CI/CD, git, deployment
- `test` — test infrastructure
- `migration` — data/schema migrations
- `cli` — CLI tools, automation

**If a task has no explicit tags**, tags will be inferred during ship-plan from
the file paths in "Files likely touched" and task text keywords. But explicit tags
are more reliable — add them when creating tasks.
