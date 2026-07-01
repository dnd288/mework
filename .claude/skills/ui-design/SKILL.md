---
name: ui-design
description: Guides the AI through UX pattern-driven UI design for user-facing OpenSpec changes. Produces ui.md as a formal design handoff — anchored to the project's existing UX patterns, abstract business flows, and visual concepts. Use when a change has a visible surface (new screens, component changes, UX improvements).
tags: [ui, frontend]
---

# UI Design

## Overview

UI design lives in `openspec/changes/<cNNNN-slug>/ui.md` — a sibling of `design.md` (architecture/logic) and `tasks.md`. But `ui.md` is not a first draft — it is a **formal design handoff document**. It captures:

- Which **UX patterns** from the project this design follows (or intentionally diverges from)
- The **abstract business flow** — the user's conceptual journey, independent of screens
- **Wireframes, component tree, and UI states** — the concrete visual design
- How the design embodies the project's **visual concepts** (character, tone, density)
- A **design handoff checklist** — confirmed done before implementation begins

A good `ui.md` makes implementation tasks concrete: each UI state becomes an acceptance criterion, each component maps to a task file, and the wireframes give the implementer an unambiguous target — all while being consistent with the project's existing UX language.

## When to Use

- Your OpenSpec change introduces or modifies user-facing screens
- You are adding or changing interactive components (forms, lists, modals, navigation, buttons)
- The change affects layout, responsive behavior, or accessibility
- You need to document UI states (loading, empty, error, edge cases)
- The human explicitly requested UI design work

**When NOT to use:** Purely backend/data-layer changes, spec-only changes, infrastructure work, CLI-only changes. If the change has no visible surface, state "no UI surface — ui.md not created" in the propose output.

## Before You Start: Every Project Is Different

Each project has its own tech stack, its own design conventions, and its own way of documenting design. Some have a `DESIGN.md`, some have design skills, some have nothing but the code itself. **Your first job is to find what already exists.**

The `ux-pattern-audit` skill (`core/skills/ux-pattern-audit/SKILL.md`) can be run separately to produce a `openspec/patterns/ux-patterns.md` — but it's not required. If it exists, read it. If it doesn't, you'll discover the patterns as part of the design process below.

## The Design Process

### Step 1: Discover the Project's Design Language

Before proposing any UI, find and read what the project already says about its design. Every project is different — adapt to what you find.

1. **Scan for existing design documentation:**
   - `DESIGN.md` or `DESIGN_SYSTEM.md` at the project root
   - `docs/design/` directory or `docs/design.md`
   - `.claude/skills/*/SKILL.md` files that reference design, UI, or UX
   - `STYLE_GUIDE.md`, `BRAND.md`, or similar
   - `openspec/patterns/ux-patterns.md` (the UX pattern reference, if it exists)
   - The project's `CLAUDE.md` for any design notes or invariants

2. **Read the relevant capability specs** — `openspec/specs/<capability>/spec.md` — to understand the intended behavior the UI must support.

3. **Read the frontend codebase** — browse the primary UI package to understand naming conventions, file organization, and how existing screens are structured.

4. **Document what you found** in the `ui.md` "Design Language Reference" section: what design docs exist, what patterns you observed, what the design language is. This anchors everything that follows.

> **If `openspec/patterns/ux-patterns.md` does not exist and this is the first UI design for the project**, consider running the `ux-pattern-audit` skill to create it. This is especially valuable for larger projects. For a small one-off change, you can document patterns inline in the `ui.md`.

### Step 2: Catalog UX Patterns

Identify which of the project's UX patterns apply to this change — and which don't. This is not about components or CSS; it's about **abstract patterns** for how the project handles common UX situations.

For each relevant pattern category, note how the existing approach applies (or doesn't) to your design:

- **Navigation** — Where does this screen live in the nav hierarchy? Does it use the project's standard nav pattern or need something new?
- **Data display** — Does your design use the project's standard card/table/list pattern? Or introduce a new data display?
- **Forms & input** — Does your change involve forms? Follow the project's established form pattern (layout, validation, submission)?
- **Feedback & notifications** — How does your design surface errors, success states, and confirmations? Use the project's existing feedback patterns?
- **Loading states** — Apply the project's established loading pattern (skeleton vs spinner) to each data-dependent region?
- **Empty states** — Follow the project's empty state convention? Or needs something new?
- **Confirmations** — Any destructive actions? Follow the project's confirmation pattern?
- **Search & browse** — Does your change involve searching or browsing? Use the existing pattern?
- **Modals / panels / pages** — If your design uses overlays or secondary views, which pattern does it follow (modal, slide-in panel, drill-down page)?

Document the pattern usage in a table:

```
| Pattern | Project Standard | Used in This Design | Notes |
|---------|-----------------|-------------------|-------|
| Navigation | sidebar + breadcrumbs | Yes — standard | — |
| Data display | card grid + slide-in detail | Card grid: yes. Slide-in: replaced with inline panel for performance |
| Forms | single-page, inline validation | Yes — standard form | — |
| Notifications | toast (top-right, auto-dismiss) | Yes — standard | — |
```

### Step 3: Map the Abstract Business Flow

Before drawing wireframes, define the **conceptual user journey** — what the user is trying to accomplish, independent of which screens or components are involved.

Map it as:

```
Goal: [what the user wants to accomplish]
Entry point: [how does the user start this journey?]
Flow steps:
  1. [conceptual step 1 — not a screen, a milestone]
  2. [conceptual step 2]
  3. [conceptual step 3]
Completion: [what "done" looks like]
What's next: [where does the user go after?]

Branch paths:
  Happy path: [step 1 → 2 → 3 → done]
  Empty: [what happens if there's nothing to act on?]
  Error: [what happens if something fails?]
  Edge cases: [timeouts, stale data, rapid actions, permissions]
```

This flow should make sense to a product manager or stakeholder without seeing any screens. It's the **business logic of the UX** — not the implementation.

**Example — don't write:**
> User clicks "New Order" button → form page loads → user fills fields → clicks Submit → table updates

**Instead write:**
> Merchant wants to create a rush order for a VIP customer who needs delivery by tomorrow. They start from the Orders dashboard, initiate a new order, select the customer and items, set rush priority, review the total, and confirm. The order appears in the active orders queue with a "rush" badge.

The second version captures the *concept* — wireframes can then realize it, but the flow survives regardless of implementation.

### Step 4: Create Wireframes / ASCII Art

Use ASCII art in fenced code blocks. Keep diagrams 72–80 characters wide. Each screen or significant state gets its own diagram.

```
+----------------------------------------------------------------------+
|  [Logo]  [Nav: Home | Search | Settings]               [Avatar v]   |
+----------------------------------------------------------------------+
|                                                                      |
|  +-----------+  +--------------------------------------------------+ |
|  | Filters   |  |  Search Results                                  | |
|  |           |  |                                                  | |
|  | [v Type]  |  |  +----------------------------------------------+ | |
|  | [ ] Active|  |  | ResultCard (title, excerpt, timestamp)      | | |
|  | [ ] Draft |  |  | [View] [Edit]                               | | |
|  |           |  |  +----------------------------------------------+ | |
|  | [Apply]   |  |  +----------------------------------------------+ | |
|  +-----------+  |  | ResultCard (title, excerpt, timestamp)      | | |
|                 |  | [View] [Edit]                               | | |
|                 |  +----------------------------------------------+ | |
|                 |                                                  | |
|                 |  [  <  Page 2 of 8  >  ]                        | |
|                 +--------------------------------------------------+ |
+----------------------------------------------------------------------+
```

Use the following conventions:
- `[Button]` or `[Label]` for interactive elements
- `[v Dropdown]` for dropdowns (v = chevron)
- `[x]` / `[ ]` for checkboxes
- `(...)` for radio buttons
- `+--...--+` for bordered containers
- `|` for vertical edges

### Step 5: Document Component Tree

Show the hierarchy of UI components using indentation. Annotate each leaf component with its possible states in brackets.

```
SearchPage
  SearchHeader
    SearchInput    [focused / blurred / filled / validation-error]
    SearchButton   [enabled / disabled / loading]
  SearchFilters
    FilterDropdown [unselected / selected / no-results]
    ToggleGroup    [all / active / archived]
  ResultList
    LoadingSpinner
    ResultCard[]   [populated / hovered / selected]
      CardTitle
      CardExcerpt
      CardMeta
      ActionMenu   [open / closed]
    EmptyState
      Illustration
      CallToAction
    ErrorState
      ErrorMessage
      RetryButton
  Pagination
    PageLink[]     [active / inactive / disabled]
```

Mark repeated elements with `[]` (e.g., `ResultCard[]`).

Annotate which components are **existing patterns** available in the project vs **new** — this helps the implementer know what to build vs what to import.

### Step 6: Enumerate UI States

For each interactive component, enumerate all relevant states:

| State | What the user sees |
|-------|-------------------|
| **Loading** | Skeleton, spinner, shimmer, or progressive placeholder while data fetches |
| **Empty** | Message + illustration + call-to-action when there's nothing to display |
| **Error** | Error message + retry button + fallback content on fetch/operation failure |
| **Populated** | The normal data-present state |
| **Disabled** | Greyed-out/inactive state while conditions aren't met |
| **Focused** | Keyboard/touch focus indicator for inputs and interactive elements |
| **Hovered** | Hover state for clickable elements |
| **Edge cases** | Long text truncation, rapid clicks, slow network, expired session, permissions denied, zero results |

Use the project's UX patterns for how these states are rendered (from Step 2): loading skeleton style, empty state format, error display pattern, etc.

### Step 7: Design User Flows (Anchored to Business Flow)

Write step-by-step flows for key scenarios, anchored to the **abstract business flow** from Step 3. This is where the conceptual flow meets actual UI.

```
## Flow: [matches Step 3's flow name]

**Happy path (following abstract flow):**
1. [UI event] → [state change, referencing business flow milestone]
2. [UI event] → [state change]

**Empty:**
- [what the user sees when business flow has no data]

**Error:**
- [what the user sees and can do when something fails]

**Edge cases:**
- [specific edge cases with expected UI behavior]
```

### Step 8: Visual Concept Alignment

Explain how this design embodies the project's visual concepts. Reference the design character, information density, tone, and visual language discovered in Step 1.

- **Character alignment** — "This design continues the project's professional/trustworthy character through clean layout and restrained color use."
- **Density** — "Follows the project's balanced approach: spacious around primary actions, denser in data-display regions."
- **Tone** — "Error messages use the project's direct, helpful tone — no blaming the user."
- **Visual language** — "Uses the established accent-color-on-neutral-base palette. New feature uses the existing icon set."

If the design intentionally diverges from an established concept, justify it:
- "Diverging from the typical spacious layout here because this power-user screen prioritizes data density — similar to how the project's dashboard already does."

### Step 9: Design Handoff Checklist

Before the design is handed to implementation (i.e., before `ship-plan` reads `ui.md`), confirm:

- [ ] Existing design docs were discovered and referenced (DESIGN.md, skills, patterns)
- [ ] UX patterns from the project are cataloged and applied (or divergence is justified)
- [ ] Abstract business flow is mapped — entry → steps → completion → next
- [ ] Wireframes cover all key screens and their significant states
- [ ] Component tree documents the full hierarchy with state annotations
- [ ] Existing vs new components are annotated
- [ ] UI states table is complete (loading / empty / error / populated / edge cases)
- [ ] User flows are anchored to the abstract business flow
- [ ] Visual concepts are referenced — character, density, tone, language
- [ ] Any divergence from established concepts is justified
- [ ] Responsive breakpoints are stated
- [ ] Accessibility considerations are noted (keyboard, ARIA, contrast)
- [ ] Implementation tasks in `tasks.md` map to `ui.md` sections
- [ ] The human has reviewed and approved the UI design before implementation begins

This checklist **must be complete** before the design is handed off.

## Template: ui.md

```markdown
# UI Design — <Feature Name>

## Overview
[One paragraph — what user-facing capability this delivers, which screens or
components it affects, who the primary user is.]

## Design Language Reference
[REQUIRED — what design documentation exists (DESIGN.md, design skill,
ux-patterns.md) and what key design language elements were found. E.g.,
"Design.md defines the primary/interactive color palette and heading hierarchy.
Existing apps use card-based layouts with slide-in detail panels."]

## UX Patterns Used
[Which established project UX patterns this design follows, with a table
mapping each pattern category to its usage in this design. Note any
intentional deviations and why.]

| Pattern | Project Standard | Used? | Notes |
|---------|-----------------|-------|-------|
| Navigation | sidebar + breadcrumbs | Yes | — |
| Data display | cards + slide-in detail | Partial | Using cards but inline panel instead of slide-in for perf reasons |
| Forms | single-page, inline validation | Yes | — |
| ... | ... | ... | ... |

## Abstract Business Flow
[The conceptual user journey — independent of screens and implementation.
Entry point → flow steps → completion → what's next. Plus branch paths
for happy, empty, error, and edge cases.]

```
Goal: [what the user wants to accomplish]
Entry: [how the user starts]
Steps:
  1. ...
  2. ...
  3. ...
Done: [what completion looks like]
Next: [where the user goes after]
```

## Wireframes / Layout
[ASCII-art diagram(s) of each screen or major component state.]

## Component Tree
[Indented hierarchy of components with state annotations. Tag each component
as [existing pattern] or [new].]

## UI States
| Component | Loading | Empty | Error | Populated | Edge Cases |
|-----------|---------|-------|-------|-----------|------------|
| SearchInput | skeleton | — | — | filled | long query |
| ResultList | spinner | no-results msg | error+retry | cards | 1000+ results |

## User Flows
[Step-by-step walkthroughs anchored to the abstract business flow above.
Cover happy path, empty, error, and edge cases.]

## Visual Concept Alignment
[How this design embodies the project's visual concepts: character,
information density, tone, visual language. Any divergence is explicitly
justified.]

## Responsive Behavior
[Breakpoints, layout changes, mobile navigation pattern. Reference the
project's established responsive patterns where applicable.]

## Accessibility Considerations
[Keyboard nav, ARIA, focus management, contrast, screen reader flow.]

## Design Handoff Checklist
- [ ] Design language referenced
- [ ] UX patterns cataloged and applied
- [ ] Abstract business flow mapped
- [ ] All UI states enumerated
- [ ] Visual concepts aligned
- [ ] Responsive behavior stated
- [ ] Accessibility covered
- [ ] Divergences from existing patterns justified

## Open UI Questions
[Anything needing human input before implementation.]
```

## Connecting to Implementation Tasks

UI design flows into implementation tasks in `tasks.md`. Each unit of work should reference `ui.md` sections and the project's UX patterns:

```markdown
## Task [3]: SearchResultList with all states (toolchain: ts)

**Description:** Build the SearchResultList component with loading skeleton,
empty-state message, error state with retry, and populated card list.
Follow ui.md → Component Tree for the hierarchy and ui.md → UI States for
the behavior of each state. Use the project's established loading state
pattern (skeleton) and empty state pattern (illustration + message + CTA).

**Acceptance criteria:**
- [ ] Loading: skeleton renders while data is being fetched (per project pattern)
- [ ] Empty: "No results found" message with suggestion (per project empty state pattern)
- [ ] Error: error message + RetryButton on fetch failure (per project error pattern)
- [ ] Populated: renders ResultCard for each result item
- [ ] Edge: handles 0, 1, and 1000+ results without layout breakage
- [ ] 1000+ results correctly paginated per ui.md → User Flows
- [ ] Responsive: layout adapts at breakpoints specified in ui.md
- [ ] Accessibility: keyboard navigation + ARIA labels per ui.md
```

## Verification

Before concluding UI design, confirm:

- [ ] Design language reference populated with what was found in the project
- [ ] UX patterns table documents reuse and deviations
- [ ] Abstract business flow is mapped (not just page-level navigation)
- [ ] Wireframes cover all key screens and their significant states
- [ ] Component tree documents the full hierarchy with state annotations, tagged existing vs new
- [ ] User flows cover happy path, empty, error, and at least one edge case
- [ ] UI states table is complete (loading / empty / error / populated / edge cases)
- [ ] Visual concept alignment is documented
- [ ] Responsive breakpoints are stated
- [ ] Accessibility considerations are noted (keyboard, ARIA, contrast)
- [ ] Design handoff checklist is complete
- [ ] Implementation tasks in `tasks.md` map to `ui.md` sections and reference UX patterns
- [ ] The human has reviewed and approved the UI design before implementation begins
