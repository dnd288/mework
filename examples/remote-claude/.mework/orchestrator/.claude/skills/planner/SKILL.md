---
name: planner
description: How to break down complex user requests into parallel, trackable work sessions. Use when the user gives you a multi-step task, a vague goal, or something that needs planning before execution.
---

# Planner

When the user gives you a complex or vague request, don't jump straight into
spawning — plan first.

## When to plan

- Request has multiple steps ("explore the repo, fix the bug, write tests")
- Request is vague ("make this better", "investigate the issue")
- Request needs research before action
- Request affects multiple areas

## The planning process

### 1. Clarify

If the request is vague, ask 1-2 targeted questions:
> "Just to make sure I understand — you want me to X, Y, and Z. Is that right?"
> "Should I focus on the backend or the frontend first?"

### 2. Break into sessions

Divide the work into independent, parallel sessions:

```
Session 1: "explore" — understand the current codebase structure
Session 2: "research" — investigate the issue
Session 3: "implement" — build the fix
```

### 3. Identify dependencies

Can sessions run in parallel, or do some depend on others?

- **Independent** → spawn all at once, monitor in parallel
- **Dependent** → spawn the dependency first, wait for it, then spawn the next

### 4. Propose the plan

Present the plan to the user before executing:
> "Here's my plan:
> 1. **explorer** — scan the repo structure (parallel)
> 2. **researcher** — investigate the bug (parallel)
> 3. **builder** — implement the fix (depends on 1+2)
>
> Sound good?"

### 5. Execute

Once approved, spawn sessions in dependency order. Track all sessions and
report results as they complete.

## Prioritization

When deciding what to work on first:
1. **Exploration** — understand the problem space before building
2. **Research** — gather information before deciding
3. **Implementation** — build after you know what to build
4. **Polish** — clean up after everything works
