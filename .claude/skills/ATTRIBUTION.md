# Skill attribution

The engineering-practice skills in this directory are **adapted from**
[addyosmani/agent-skills](https://github.com/addyosmani/agent-skills) (MIT License),
a library of production-grade engineering skills for AI coding agents.

Each skill keeps the upstream methodology and structure, generalized for use with
the mzspec OpenSpec ship pipeline (the `/opsx:*` spec-first two-PR lifecycle,
resolver-selected gates, and evidence under `openspec/changes/<name>/evidence/`).
Project-specific details (toolchains, hard-invariants) are read from the consuming
repo's `mzspec.config.json` rather than hardcoded; where a skill shows concrete
examples (e.g. `security-and-hardening`), those are the MeKnow reference set and
should be swapped for your project's invariants.

## Vendored from upstream (adapted)

- `using-agent-skills` — meta-router / operating rules
- `spec-driven-development` — mapped onto OpenSpec
- `planning-and-task-breakdown`
- `test-driven-development`
- `incremental-implementation`
- `debugging-and-error-recovery`
- `code-review-and-quality`
- `code-simplification`
- `security-and-hardening`
- `git-workflow-and-versioning`
- `ci-cd-and-automation`
- `documentation-and-adrs`

Upstream license: MIT, © Addy Osmani and contributors. This adaptation retains the
MIT terms; see the upstream `LICENSE` for the full text.
