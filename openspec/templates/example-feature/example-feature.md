---
name: example-feature
description: Example template — a vertical feature touching one package (delete or adapt for your project)
---

# Example: planning a single-package feature

This is a **starter** template shipped by mzspec to show the shape. Delete it, or adapt
it to a kind of change your project actually plans often. It applies when a change adds
one user-visible capability inside a single toolchain package (not a cross-cutting
migration).

## How to plan the tasks

- **One task per seam, not per file.** Split along contract → implementation → docs:
  1. *Spec/contract* — the data shape / API / interface the feature exposes.
  2. *Implementation (test-first)* — the production change, with the failing tests it
     must make pass listed as the task's deliverable.
  3. *Docs/changelog* — only if the change is user-visible.
- **Landscape-aware:** read the package's toolchain in `mzspec.config.json`; the
  implementation task's gates are exactly that toolchain's gates (don't invent new ones).
- **Each task carries:** a single deliverable, its `depends_on`, and an acceptance line
  whose check is one of the gates the resolver will emit for the touched paths.
- **YAGNI:** no task for absent scope (no migration task if nothing under `migrations/`
  changes; no docs task for an internal-only change).

> Replace this guide with your project's real planning playbook, or remove this folder
> if you don't want a starter template (`/opsx:template-remove example-feature`).
