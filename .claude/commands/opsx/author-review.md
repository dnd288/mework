---
name: "OPSX: Author Review"
description: Automated multi-dimension code review for team leads — posts inline PR review comments, reviewee picks them up with address-review
category: Workflow
tags: [workflow, automation, review, pr, feedback, leadership]
---

Automated code review for team leads. Two modes:

**Review mode** (default): Fetches the PR diff, runs parallel review agents across
selected dimensions, posts findings as inline PR review comments — the reviewee
picks them up with `/opsx:address-review`.

**Comment mode** (`--comment`): Posts the leader's custom message as a PR review
comment. Skips AI entirely. Useful when the leader already knows what to ask for.

**Input**: A change name (e.g. `/opsx:author-review c0005-llm-profiling-inspector`)
or a PR URL/number (e.g. `/opsx:author-review --pr 10`).

**Options**
- `--dimensions correctness,security,quality,spec` — which review axes to run (default: all)
- `--dry-run` — collect findings but do NOT post to the PR
- `--pr <url|number>` — review a PR directly (no OpenSpec change needed)
- `--prompt "<text>"` — custom instruction injected into EVERY review dimension
- `--comment "<text>"` — **comment mode**: skip AI review, just post this message
  - `--commentFile <path>` — (optional) file path for inline comment
  - `--commentLine <n>` — (optional) line number for inline comment

**Examples**

```bash
# Full AI review (all 4 dimensions)
/opsx:author-review c0005-llm-profiling-inspector

# Single dimension
/opsx:author-review c0005-llm-profiling-inspector --dimensions security

# Custom prompt
/opsx:author-review c0005-llm-profiling-inspector --prompt "Check error handling paths"

# Comment mode — just post a message, no AI
/opsx:author-review c0005-llm-profiling-inspector --comment "Please add unit tests for the new extractor function"

# Inline comment on a specific file/line
/opsx:author-review c0005-llm-profiling-inspector --comment "This needs null guard" --commentFile src/extraction.py --commentLine 42

# Direct PR (no change slug)
/opsx:author-review --pr 10 --dimensions correctness,security
```

**Steps (review mode)**

1. **Select the target.** Change slug → find open PR. `--pr` → use directly.
2. **Launch the Workflow:**
   ```
   Workflow({ name: 'author-review', args: { change: '<name>', dryRun: <bool?>, dimensions: [...], prompt: '<...>', base: '<base?>' } })
   ```
3. **Review runs in parallel** across requested dimensions:
   - **correctness** — logic bugs, off-by-one, race conditions, error handling
   - **security** — injection, PII leakage, input validation, secrets
   - **quality** — design, DRY, naming, complexity, test coverage
   - **spec** — compliance with the spec contract (OpenSpec only)
4. **Post findings** as inline PR review comments with severity labels.
5. **Relay result** — lead checks + adds notes, reviewee runs `address-review`.

**Steps (comment mode)**

1. **Select the target** (same as above).
2. **Launch with `--comment`** — skips straight to posting the leader's message.
3. **Relay result** — reviewee runs `address-review` to pick up the comment.

**Guardrails**
- Never approves or merges the PR — a human lead always has the final say.
- Findings are posted as **COMMENT** reviews, not REQUEST_CHANGES.
- `--dry-run` collects + prints findings without posting.
- `--comment` + `--dry-run` is rejected (nothing to dry-run).
- If posting fails, the findings summary is still logged for manual use.
