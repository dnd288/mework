---
name: openspec-sync-specs
description: Merges a change's delta specs into the canonical openspec/specs/ tree. Idempotent — re-running is a no-op.
tags: []
---

# Openspec Sync Specs

Merges a change's delta specs into the canonical `openspec/specs/` tree.
Idempotent — re-running is a no-op.

For each delta spec file under `openspec/changes/<change>/specs/`:

1. **`# ADDED <capability>/spec.md`** — copy the spec into `openspec/specs/<capability>/spec.md`
2. **`# MODIFIED <capability>/spec.md`** — merge changes into `openspec/specs/<capability>/spec.md`
3. **`# REMOVED <capability>/spec.md`** — remove `openspec/specs/<capability>/spec.md`
4. **`# RENAMED <old> <new>`** — move the canonical spec

Use `mv`/`cp`/`rm` for the file operations. Always validate after sync:

```
node .claude/workflows/lib/openspec.js validate "<change>" --strict
```
