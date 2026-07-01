---
name: ux-pattern-audit
description: Discovers and documents a project's UX language — the abstract patterns, business flows, and visual concepts that make its design consistent. Use before creating the first ui.md in a project, when making UX-significant changes, or when a human requests a design audit. Outputs openspec/patterns/ux-patterns.md.
tags: [ui, design]
---

# UX Pattern Audit

## Overview

A project's UX consistency comes from its **patterns** — the recurring solutions to common UX situations — not from its CSS variables or component library. This skill discovers and documents those patterns at an abstract, tech-stack agnostic level, producing a **UX Pattern Reference** (`openspec/patterns/ux-patterns.md`) that all future `ui.md` files anchor to.

Unlike a component library or design token file, the UX Pattern Reference captures:
- **UX patterns** — how the project handles navigation, forms, errors, loading, empty states, etc. — independent of any specific UI framework
- **Abstract business flows** — the conceptual user journey and mental model, not page-level navigation
- **Visual concepts** — the high-level design character, tone, and visual language

Every project already has some form of design documentation (a `DESIGN.md`, a design skill, a design system doc, or conventions embedded in the codebase). The audit finds and synthesizes what's already there — it doesn't invent a new design system.

## When to Use

- Before creating the first `ui.md` in a project (to establish the UX baseline)
- When starting a change that introduces new UX patterns or significantly modifies existing ones
- When the project's `openspec/patterns/ux-patterns.md` is missing or stale
- When a human requests "audit the current UX patterns" or "document the design language"
- When onboarding to a new or unfamiliar project

**When NOT to use:** Purely backend changes, infrastructure work, CLI-only changes, or any change with no user-facing surface. Also skip if the pattern reference already exists and is not stale — just reference it during `ui-design`.

## The Audit Process

### Step 1: Discover Existing Design Documentation

Before extracting anything from the codebase, find and read what the project already says about its design. Every project is different — some have extensive design docs, others have none.

1. **Scan for design documentation files:**
   - `DESIGN.md` or `DESIGN_SYSTEM.md` at the project root
   - `docs/design/` directory or `docs/design.md`
   - `.claude/skills/*/SKILL.md` files that reference design, UI, or UX
   - `STYLE_GUIDE.md` or `STYLE.md`
   - `BRAND.md` or `BRAND_GUIDELINES.md`
   - Any `README.md` sections about design or UI conventions
   - `openspec/patterns/` directory (existing pattern reference)
   - `openspec/specs/` — capability specs describe intended UX behavior

2. **Read the project's CLAUDE.md** — it may contain design notes, invariants, or references to design skills.

3. **Note the tech stack** (context, not the focus): React/Vue/Svelte/SwiftUI? Tailwind/CSS Modules? shadcn/ui, Radix, MUI, or custom? Mobile-first or desktop-first? This tells you where to look for patterns but isn't the output.

4. **Document what you found:** "Found DESIGN.md at root with color/typography guidelines. CLAUDE.md references a `ui-design` skill. No existing pattern catalog."

### Step 2: Extract UX Patterns

For each common UX situation, catalog how *this specific project* handles it. Focus on the **abstract pattern**, not the implementation:

- **Navigation** — Sidebar, topnav, tabs, or hybrid? Breadcrumbs? How deep does the nav hierarchy go? Is there a persistent nav or context-sensitive? How does the user know where they are?

- **Data display** — Tables, cards, lists, or a mix? When is each used? How is detail accessed — inline expansion, drill-down page, slide-in panel, modal? Sorting, filtering, pagination?

- **Forms & input** — Single-page or multi-step/wizard? How is validation shown (inline, tooltip, top-of-form banner)? Save button vs auto-save? How are errors surfaced per field? Confirmation before submit?

- **Feedback & notifications** — Toasts, banners, inline messages, or a notification center? How long do they persist? Can the user dismiss them? Error display pattern — modal, inline, or dedicated error page?

- **Loading states** — Skeleton screens, spinners, shimmer, or progress bars? Where and when is each used? Is there a loading state for every data-dependent region?

- **Empty states** — Illustration + message + CTA? Or just "no results" text? Is there a pattern for suggesting next actions? When is a blank slate shown vs hidden?

- **Confirmations** — Destructive action confirmations (delete, cancel)? Pattern: simple confirm dialog, double-click, undo toast, or no confirmation? Where is confirmation required vs skipped?

- **Search & browse** — Filter sidebar vs inline filters vs search-first? Autocomplete? Faceted search? How are results displayed and paginated?

- **Authentication & onboarding** — Login flow, signup flow, password reset, permission screens, onboarding wizard. How does the app introduce itself to new users?

- **Responsive behavior** — Mobile-first or desktop-first? How does navigation collapse? What happens to sidebars, tables, multi-column layouts on small screens?

- **Error & edge case patterns** — 404, 403, network error, stale data, expired session. Is there a consistent error recovery pattern?

- **Modals & dialogs** — When are modals used vs inline panels vs full pages? Size conventions? Close on escape/click-outside? Trap focus?

Document each as a named, reusable pattern with a clear description. A future `ui.md` should be able to say "uses the project's standard form pattern" and that should mean something specific.

```
## UX Patterns

### Navigation: sidebar + topnav hybrid
- Primary nav in sidebar (collapsible), secondary in top bar
- Breadcrumbs below top bar for depth > 1
- Active section highlighted in sidebar, subsection in topnav
- Current page reflected in both

### Data display: card grid + slide-in detail
- Browse views use card grids (3 columns on desktop, 2 tablet, 1 mobile)
- Clicking a card opens a slide-in panel from the right (not a new page)
- Panel has back button, not a close button
- ...etc
```

### Step 3: Map Abstract Business Flows

This is the most important step. Map the **conceptual user journey** — not page-level navigation, but the business task the user is trying to accomplish.

For the project's primary user goals, describe:

```
Goal: [what the user wants to accomplish]
Entry point: [where does this journey start?]
Flow steps: [the conceptual steps, not the screens]
  1. ...
  2. ...
  3. ...
Completion: [what "done" looks like]
What's next: [where does the user go after?]
Branch paths:
  - Happy path: ...
  - Empty: ...
  - Error: ...
  - Edge: ...
```

Keep flows abstract enough that they don't change when the UI moves from one tech stack to another. A good test: the flow should still make sense if you described it to a product manager without showing any screens.

### Step 4: Define Visual Concepts

Capture the high-level design language — the things you'd notice walking through the app that don't fit into a specific pattern:

- **Character** — "professional and trustworthy" / "playful and vibrant" / "minimal and technical" / "friendly and approachable" / "premium and polished"

- **Information density** — "spacious and airy with lots of whitespace" vs "dense data-rich with minimal chrome" vs "balanced — dense where needed, spacious where not"

- **Visual language** — "bold color accents on a neutral base" / "monochromatic with restrained use of color" / "full-color illustrations and photography" / "minimalist with strong typography" / "glassmorphism / neumorphism / flat"

- **Tone of voice** — "friendly and conversational" / "formal and precise" / "encouraging and supportive" / "direct and no-nonsense" / "playful with micro-copy"

- **Key UX principles** — this project values: "progressive disclosure — reveal complexity gradually" / "everything visible at once — no hidden menus" / "escape hatch always available" / "undo over confirm" / "optimistic updates with rollback" / "fail gracefully with clear next steps"

- **Design personality** — If this app were a person, what would they be like? This helps new designers (and AI) internalize the gestalt more than any token list.

### Step 5: Write the UX Pattern Reference

Write the complete reference to `openspec/patterns/ux-patterns.md`. Use the following template:

```markdown
# UX Pattern Reference — [Project Name]

## Design Documentation Found
[What existing design documentation was discovered (DESIGN.md, skills, etc.)]

## Tech Stack Context
[Brief note on tech stack — for context only, not a constraint]

## Visual Concepts
- **Character:** ...
- **Information density:** ...
- **Visual language:** ...
- **Tone of voice:** ...
- **Key UX principles:** ...
- **Design personality:** ...

## UX Patterns
### Navigation
### Data Display
### Forms & Input
### Feedback & Notifications
### Loading States
### Empty States
### Confirmations
### Search & Browse
### Authentication & Onboarding
### Responsive Behavior
### Error & Edge Cases
### Modals & Dialogs

## Abstract Business Flows
### Flow 1: [Primary goal]
### Flow 2: [Secondary goal]
### ...

## Appendix: Screenshots / References
[Optional — key screenshots or file references that illustrate the patterns]
```

If a pattern category isn't applicable or no consistent pattern was found, note it: "No consistent pattern found — varies by context" rather than omitting it.

## Refreshing an Existing Reference

When the pattern reference already exists but may be stale:

1. Read the existing `openspec/patterns/ux-patterns.md`
2. Walk through the current codebase to verify each documented pattern still holds
3. Note any divergences: "Pattern X has shifted — previously used modals, now uses slide-in panels"
4. Update the reference with the current state
5. Mark what changed: "Updated: Navigation pattern, Responsive behavior. Added: New flow for feature Y"

## Verification

Before concluding the audit:

- [ ] DESIGN.md / design skills / existing docs were discovered and read
- [ ] All relevant UX pattern categories are populated (or explicitly noted as N/A)
- [ ] Patterns are described abstractly — no implementation-specific terminology
- [ ] At least one abstract business flow is mapped for the project's primary user goal
- [ ] Visual concepts are defined (character, density, language, tone, principles)
- [ ] The output is written to `openspec/patterns/ux-patterns.md`
- [ ] If refreshing: changes from the previous version are documented
