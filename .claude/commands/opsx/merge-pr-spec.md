---
name: "OPSX: Merge SPEC PR"
description: Merge a spec/ branch (contract). Preflight, merge, fire after-spec-pr-merged lifecycle — no archive or changelog.
category: Workflow
tags: [workflow, merge, pr, spec, contract]
---

Merge a **spec PR** (`spec/<change>` branch) — the contract-only PR that locks the spec
before implementation. Runs:

1. **Preflight**: checks PR is OPEN, not draft, no conflicts, on a `spec/` branch
2. **Prepare** (optional): update PR title/body
3. **Merge**: via GitHub API (squash/merge/rebase), deletes branch
4. **Hooks**: fires `after-spec-pr-merged` lifecycle — comments on linked issue,
   advances status, updates Projects board

No archive (spec changes aren't archived) and no changelog check.

**Input**: A change slug (`c0012-rest-api-kb-management`) or PR URL/number.

**Options**
- `--pr <url|number>` — merge a PR directly (auto-detects branch)
- `--title "<text>"` — override PR title before merge
- `--body "<text>"` — replace/append PR body
- `--strategy squash|merge|rebase` — default: squash
- `--dry-run` — check only, no merge

**Examples**
```
/opsx:merge-pr-spec c0015-remove-overview-messages-placeholder
/opsx:merge-pr-spec --pr 92 --title "spec(c0015): remove placeholder chart"
/opsx:merge-pr-spec c0015-remove-overview-messages-placeholder --dry-run
```

**Guardrails**
- Only accepts `spec/<change>` branches. If the branch is `feat/`, use `merge-pr-code`.
- Never merges draft or conflicting PRs.
- Does NOT archive the change (spec PRs only lock the contract).
