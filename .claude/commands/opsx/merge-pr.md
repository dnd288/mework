---
name: "OPSX: Merge PR"
description: Auto-dispatch — detects whether the PR is spec/ or feat/ and routes to the dedicated workflow
category: Workflow
tags: [workflow, merge, pr, github]
---

**Auto-dispatch** merge workflow. Detects whether the PR branch is `spec/<change>` or
`feat/<change>` and delegates to the dedicated workflow:

| Branch | Routes to | Does what |
|---|---|---|
| `spec/<change>` | `/opsx:merge-pr-spec` | Preflight → Prepare → Merge → Lifecycle (`after-spec-pr-merged`). No archive/changelog. |
| `feat/<change>` | `/opsx:merge-pr-code` | Preflight → Prepare → Archive → Merge + close issues → Lifecycle (`after-code-pr-merged`). |

Use this as the default entrypoint. For explicit control, call the dedicated workflow
directly.

**Input**: A change slug (`/opsx:merge-pr c0012-rest-api-kb-management`)
or a PR URL/number (`/opsx:merge-pr --pr 75`).

**Options** (forwarded to the sub-workflow)
- `--pr <url|number>` — merge a PR directly (auto-detects branch from PR headRefName)
- `--title "<text>"` — override PR title
- `--body "<text>"` — replace/append PR body
- `--closes "#N"` — add `Closes #N` (code PR only)
- `--strategy squash|merge|rebase` — default: squash
- `--skip-archive` — skip archive (code PR only)
- `--dry-run` — check only, no merge

**Examples**
```
/opsx:merge-pr c0012-rest-api-kb-management         # auto-detect spec/ or feat/
/opsx:merge-pr --pr 75                               # auto-detect from PR branch
/opsx:merge-pr c0012-rest-api-kb-management --dry-run # dry run
```

**Guardrails**
- Delegates based on branch prefix. If detection fails, reports the error.
- All sub-workflow guardrails apply (no draft, no conflicts, etc.).
- For explicit control, use `/opsx:merge-pr-spec` or `/opsx:merge-pr-code` directly.
