---
name: "OPSX: Merge CODE PR"
description: Merge a feat/ branch (implementation). Preflight, archive, merge, close issues, fire after-code-pr-merged lifecycle.
category: Workflow
tags: [workflow, merge, pr, implementation, archive]
---

Merge a **code PR** (`feat/<change>` branch) — the implementation PR with the actual
changes. Runs:

1. **Preflight**: checks PR status, detects linked issues, checks CHANGELOG entry
2. **Prepare** (optional): update PR title/body, add `Closes #N` keywords
3. **Archive**: archives the OpenSpec change as a commit on the branch **before** merge
4. **Merge**: merges via GitHub API (squash/merge/rebase), closes linked issues,
   deletes branch
5. **Hooks**: fires `after-code-pr-merged` lifecycle — comments on linked issue,
   moves Projects board to **Done**, closes the issue

**Input**: A change slug (`c0012-rest-api-kb-management`) or PR URL/number.

**Options**
- `--pr <url|number>` — merge a PR directly (auto-detects branch)
- `--title "<text>"` — override PR title before merge
- `--body "<text>"` — replace/append PR body
- `--closes "#N"` — add `Closes #N` to PR body (comma-separated for multiple)
- `--strategy squash|merge|rebase` — default: squash
- `--skip-archive` — skip the auto-archive step
- `--dry-run` — check only, no merge

**Examples**
```
/opsx:merge-pr-code c0012-rest-api-kb-management
/opsx:merge-pr-code c0012-rest-api-kb-management --title "feat(c0012): add KB REST API"
/opsx:merge-pr-code --pr 88 --closes "#74, #75"
/opsx:merge-pr-code c0012-rest-api-kb-management --dry-run
```

**Guardrails**
- Only accepts `feat/<change>` branches. If the branch is `spec/`, use `merge-pr-spec`.
- Never merges draft or conflicting PRs.
- Archives the change before merging (commit on the branch, included in the PR).
- Auto-closes linked issues after merge.
- Add a CHANGELOG entry before merging if missing.
