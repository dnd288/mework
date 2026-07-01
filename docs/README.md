# MeWork Documentation

**MeWork** is an AI-agent runtime + provider-gateway. It connects task-management
platforms (Mello kanban today; Jira / Linear / GitHub Issues by design) to AI coding
CLIs (Claude Code, Codex, OpenCode) that run **locally** on a developer's machine —
so source code and provider credentials never leave machines you control.

These docs describe the project's **target architecture** — a GitHub-Actions-runner-style
**agent hub** — as the canonical product, and use status badges to mark what is built
today versus planned:

- **`[Implemented]`** — shipped in the current codebase.
- **`[Planned — cNNNN]`** — specified under `openspec/changes/cNNNN-*`, not yet built.

The system is mid-migration from a poll/queue model (built) to the agent hub (planned).
Every architecture doc carries a **Today → Target** view so you can tell the two apart.

## Reading paths

**I want to use MeWork (developer on a board):**
1. [product-overview.md](product-overview.md) — what it is, how a run works.
2. [cli-and-usage.md](cli-and-usage.md) — install, configure, trigger an agent from a ticket.

**I want to operate the server:**
1. [deployment-guide.md](deployment-guide.md) — deploy `mework-server` (env, systemd, Docker, backups).
2. [auth-and-secrets.md](auth-and-secrets.md) — tokens, credential sealing, required env vars.

**I want to understand or extend the system (developer/architect):**
1. [philosophy.md](philosophy.md) — design principles and invariants.
2. [architecture.md](architecture.md) — the agent hub, the Today → Target migration, the roadmap.
3. [api-reference.md](api-reference.md) — every endpoint, auth, topics, wire schema, data model.
4. [runtime-and-sandbox.md](runtime-and-sandbox.md) — the runner loop and pluggable sandboxes.
5. [auth-and-secrets.md](auth-and-secrets.md) — the full authentication and secrets model.

**I want to test the system (interface-first E2E):**
1. [../tests/e2e/README.md](../tests/e2e/README.md) — the BDD E2E scenario suite.
2. [../tests/e2e/SCENARIOS.md](../tests/e2e/SCENARIOS.md) — every scenario mapped to spec + doc.

**I want to contribute (process):**
1. [openspec-workflow.md](openspec-workflow.md) — spec-driven development with OpenSpec.
2. [engineering-skills.md](engineering-skills.md) — the SDD + TDD engineering-practice skills.

## Document map

| Doc | Purpose | Audience |
|-----|---------|----------|
| [product-overview.md](product-overview.md) | What MeWork is, the problem, the run lifecycle | Everyone |
| [philosophy.md](philosophy.md) | Design principles and non-negotiable invariants | Developer |
| [architecture.md](architecture.md) | Agent-hub architecture, Today → Target, roadmap | Developer / architect |
| [api-reference.md](api-reference.md) | HTTP + SSE endpoints, auth, topics, schema, data model | Developer |
| [runtime-and-sandbox.md](runtime-and-sandbox.md) | Runner lifecycle + pluggable sandbox drivers | Developer |
| [auth-and-secrets.md](auth-and-secrets.md) | Auth model, tokens, grants, sealing, env, perms | Developer / operator |
| [cli-and-usage.md](cli-and-usage.md) | CLI reference + end-to-end usage walkthrough | User / developer |
| [deployment-guide.md](deployment-guide.md) | Deploy and operate `mework-server` | Operator |
| [openspec-workflow.md](openspec-workflow.md) | Spec-driven development workflow | Contributor |
| [engineering-skills.md](engineering-skills.md) | SDD + TDD engineering-practice skills | Contributor |
| [../tests/e2e/](../tests/e2e/README.md) | Interface-first BDD E2E scenario suite | Developer / contributor |

The authoritative source is always the code plus the `openspec/specs/` baseline specs
and `openspec/changes/` proposals. Where a doc and the code disagree, the code wins —
please open a change to fix the doc.
