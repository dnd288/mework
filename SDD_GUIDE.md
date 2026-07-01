# Spec-Driven Development with mzspec

This project uses **[mzspec](https://github.com/minhlucncc/mzspec)** — a spec-driven delivery
pipeline built on top of [OpenSpec](https://github.com/Fission-AI/OpenSpec). Non-trivial work flows
**task → spec → ship**, gated by two human-reviewed PRs. This guide is the 2-minute orientation; the
commands live under `/opsx:*`.

## The pipeline

```
  task ──▶ spec ──▶ ship
   │         │        │
   │         │        └─ implement test-first, verify gates, open the CODE PR (human merges)
   │         └─ write & review the spec, open the SPEC PR (human merges → contract on main)
   └─ pick the next backlog item (or author one) and turn it into a change
```

Nothing reaches `main` without a human merging the spec PR **and** the code PR. The agent never
self-merges.

## 1. Task — get something to work on

Start straight from a prompt, or — with the **task-github** extension installed
(`./mzspec install task-github`) — from a GitHub issue:

| Command | What it does |
|---|---|
| `/opsx:propose <what>` | scaffold a change `cNNNN-<slug>` + draft `proposal.md` from a prompt (no task coupling) |
| `/opsx:propose-gh <issue>` | start from a GitHub issue → scaffold + link (`github.json`) + mark the issue **in-progress** *(task-github)* |
| `/opsx:task-log --text "…"` | comment on the linked GitHub issue *(task-github)* |
| `/opsx:task-assign [<login>]` | assign the linked issue (defaults `@me`) *(task-github)* |

## 2. Spec — define the contract before code

| Command | What it does |
|---|---|
| `/opsx:propose` | create the change + draft `proposal.md` / `design.md` / `tasks.md` / delta specs |
| `/opsx:spec` | review the spec across 6 axes (structure, clarity, testability, minimality, consistency, completeness) until it's APPROVE |
| `/opsx:spec-pr` | sync the delta specs into the canonical `openspec/specs/` and open the **SPEC PR** — a human merges it so the contract is on `main` |

## 3. Ship — implement against the merged contract

| Command | What it does |
|---|---|
| `/opsx:ship-plan` | group the change into a few test-first work-units under `.handoff/<change>/` |
| `/opsx:ship-code` | implement unit-by-unit (Red → Green → one commit), run the resolver-selected gates, write evidence, open the **CODE PR** (human merges) |
| `/opsx:address-review` | address human PR feedback, re-run gates, push, reply |
| `/opsx:archive` | after the code PR merges, archive the change |

`/opsx:ship` runs plan → code in one go.

## Where things live

- `openspec/specs/` — the canonical capability specs (the contract).
- `openspec/changes/<change>/` — in-progress change: `proposal.md`, `design.md`, `tasks.md`, delta
  `specs/`, `evidence/`, and (with task-github) `github.json` (the link back to the GitHub issue).
- `openspec/changes/archive/` — shipped changes.
- `.handoff/<change>/` — the test-first execution plan (git-ignored).
- `mzspec.config.json` — this project's config: toolchains + quality **gates**, `taskSources`, and
  hard `invariants`. Edit this, not the vendored `.claude/` files.

## Quality gates

On every change, `ship-code` resolves which gates to run from the touched files
(`.claude/workflows/lib/gate-resolver.js` reading `mzspec.config.json`) — per-toolchain lint/type/test
plus any project gates — and each must pass before the PR. See `.claude/mzspec-gates/CONTRACT.md`.

## Quick start

```
/opsx:propose <what>            # scaffold a change from a prompt
                                # (or /opsx:propose-gh <issue> from a GitHub issue)
/opsx:spec <change>             # review the spec
/opsx:spec-pr <change>          # open the spec PR  → (human merges)
/opsx:ship-plan <change>        # plan the work-units
/opsx:ship-code <change>        # implement + open the code PR → (human merges)
/opsx:archive <change>
```

Full docs: [task-github](https://github.com/minhlucncc/mzspec/blob/main/docs/task-github.md) ·
[architecture](https://github.com/minhlucncc/mzspec/blob/main/docs/architecture.md) ·
[customize](https://github.com/minhlucncc/mzspec/blob/main/docs/customize.md).
