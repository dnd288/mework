---
name: tag-system
description: Documents the deterministic tag-driven skill/hook routing system. Tags on tasks and units determine which skills and hooks are loaded at each pipeline phase — no conditional text needed. Use to understand how tags work, what tags exist, and how to tag tasks.
tags: [design]
---

# Tag System

## Overview

The tag system replaces static "if this unit touches UI" conditional text in prompt hooks with **deterministic tag matching**. Every task and unit carries tags that describe what kind of work it is. Every skill and hook declares which tags it applies to. The tag resolver matches them at runtime — only relevant guidance reaches the agent.

**Without tags:**
```
Hook says: "If this unit touches UI, do X. If backend, ignore."
(Agent must read and follow conditional logic — fragile)
```

**With tags:**
```
Unit has tags: [ui, form]
Hook has tags: [ui]
Resolver: tags match → hook loaded. Backend units never see it.
(Pure deterministic matching — no conditionals)
```

## Core Tag Taxonomy

These are the standard tags recognized by the system. Projects can add their own.

| Tag | When to Use | Skills It Activates |
|-----|-------------|-------------------|
| `ui` | Components, screens, forms, any user-facing surface | ui-design, ux-pattern-audit |
| `frontend` | Frontend code (JS/TS/CSS/HTML) | ui-design |
| `backend` | Server-side logic, APIs, services | — (universal skills apply) |
| `api` | API endpoints, contracts, serialization | — |
| `db` | Database schemas, queries, migrations | — |
| `data` | Data processing, transforms, ETL | — |
| `auth` | Authentication, authorization, permissions | security-and-hardening |
| `security` | Security hardening, vulnerability fixes | security-and-hardening |
| `config` | Configuration, environment, setup | — |
| `docs` | Documentation, ADRs, guides | documentation-and-adrs |
| `infra` | Infrastructure, CI/CD, git, deployment | git-workflow-and-versioning, ci-cd-and-automation |
| `test` | Test infrastructure, fixtures, test tooling | test-driven-development |
| `migration` | Data/schema migrations | — |
| `cli` | CLI tools, scripts, automation | — |
| `design` | UX design artifacts, wireframes, patterns | ux-pattern-audit |

**Universal skills** (no tags — always loaded): `spec-driven-development`, `spec-review-and-quality`, `planning-and-task-breakdown`, `code-review-and-quality`, `code-simplification`, `incremental-implementation`, `debugging-and-error-recovery`, `orchestrator`

## How to Tag Tasks

In `tasks.md`, annotate each task with its tags:

```markdown
## Task [1]: Login form UI  (tags: ui, auth)

## Task [2]: Token validation API  (tags: backend, api, security)

## Task [3]: User table migration  (tags: db, migration)

## Task [4]: Setup CI pipeline  (tags: infra)
```

If a task has no explicit tags, the system infers them from the file paths in its "Files likely touched" section, using project-specific tag→path mappings from `mzspec.config.json`.

### Tagging Guidelines

- **Be specific** — a task that touches both UI and backend should list both: `(tags: ui, backend)`
- **Don't over-tag** — `(tags: ui, frontend)` is redundant since `ui` implies frontend. Pick the most descriptive.
- **Default for unspecified** — if unsure, omit tags. The unit will still get universal skills.
- **Per-project tags** — projects can define custom tags in `mzspec.config.json` → `tags.categories`

## How Skills Declare Tags

Each `SKILL.md` has a `tags` field in its YAML frontmatter:

```yaml
---
name: ui-design
description: Guides UI design...
tags: [ui, frontend]
---
```

- **tags** — the kinds of units this skill is relevant for
- **No tags field** — the skill is universal (always loaded, like methodology skills)
- **Tag `[all]`** — treated the same as no tags (universal)

## How Hooks Declare Tags

Each `.prompt.md` hook file has a `tags` field in its YAML frontmatter:

```markdown
---
tags: [ui]
---

## Content here — only injected when unit has `ui` tag
```

## How the Tag Resolver Works

The `tag-resolver.js` module (`lib/tag-resolver.js`) uses simple set logic:

1. **SKILL.md with no `tags` field** → always returned (universal)
2. **SKILL.md with `tags: [ui, frontend]`** → only returned when the unit has BOTH `ui` AND `frontend` tags
3. **Hook with no `tags` field** → always injected (universal)
4. **Hook with `tags: [ui]`** → only injected when the unit has `ui` tag
5. **Multiple matches** → all matching skills/hooks are returned (union)
6. **No matches** → only universal skills/hooks are returned

### Tag Inference from File Paths

When a unit has code deliverables (file paths) but no explicit tags, `classifyTags()` infers tags from path prefixes:

```json
{
  "tags": {
    "categories": {
      "ui": { "paths": ["apps/portal/", "apps/web/"] },
      "backend": { "paths": ["packages/", "apps/api/"] },
      "db": { "paths": ["migrations/", "prisma/"] }
    }
  }
}
```

A unit delivering `apps/portal/src/SearchPage.tsx` would auto-infer `ui` tag.

## Unit-to-Unit Flow

```
1. PROPOSE: Tasks created with tags (or inferred from paths)
2. SHIP-PLAN: Tags inherited from covered tasks + code file paths
3. SHIP-CODE: Per unit, resolver loads matching skills + hooks
```

If a unit covers tasks ["1", "3"], where task 1 has tags `[ui, auth]` and task 3 has `[backend]`, the unit's tags are the union: `[ui, auth, backend]`.

## Custom Project Tags

Projects can extend the tag taxonomy in `mzspec.config.json`:

```json
{
  "tags": {
    "categories": {
      "billing": { "paths": ["apps/billing/"] },
      "ml": { "paths": ["packages/ml/"] }
    }
  }
}
```

Then create a `SKILL.md` for the custom skill with `tags: [billing]` and place it in the project's `.claude/skills/` directory. The resolver will discover and match it automatically.
