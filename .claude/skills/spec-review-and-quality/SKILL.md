---
name: spec-review-and-quality
description: Six-axis spec review methodology — Structure, Clarity, Testability, Minimality, Consistency, Completeness, and UX Pattern Consistency. Each axis is evaluated independently, then findings are consolidated into a review report.
tags: []
---

# Spec Review and Quality

Six-axis spec review methodology. Each axis is evaluated independently,
then findings are consolidated into a review report.

## Axes

1. **Structure & validity** — `openspec validate --strict` passes; Purpose + Requirements sections;
   SHALL/MUST in the body line; >=1 Scenario each; delta uses ADDED/MODIFIED/REMOVED/RENAMED
   with the FULL requirement on MODIFY
2. **Clarity / KISS** — unambiguous, no jargon, one concept per sentence
3. **Testability** — every requirement has a clear pass/fail test scenario
4. **Minimality / YAGNI** — no scope beyond what's stated, no speculative features
5. **Consistency / DRY** — terminology is consistent with existing specs, no duplication
6. **Completeness** — no dangling references, all edge cases addressed; for UI changes, `ui.md` must exist with loading/empty/error states documented, a "Design Language Reference" section citing project design docs (or noting their absence), and UX patterns referenced from the project's design conventions
7. **UX Pattern Consistency** — for user-facing changes: `ui.md` references the project's existing UX patterns (from DESIGN.md, design skills, `openspec/patterns/ux-patterns.md`, or codebase exploration) or explicitly justifies divergence; the abstract business flow is coherent and maps entry → steps → completion → next; visual concepts (character, density, tone) align with the project's established design language; `ui.md` reads as a final design handoff, not a first draft

## Process

1. Run `node .claude/workflows/lib/openspec.js validate "<change>" --strict`
2. Review each axis independently
3. Fix Blocker/Required findings, re-validate
4. Write REVIEW.md with verdict and findings

Severity: Blocker (invalid/untestable/contradicts invariant) > Required (fix before ship) > Nit (optional) > FYI.
