---
name: "spawn"
description: "Spawn a new work session / child sandbox for a task"
category: Orchestrator
tags: [spawn, create, session, sandbox, delegate]
---

Spawn a child sandbox to work on a task. Usage:

`/spawn <description of the task>`

1. Assess the task — is it clear enough to delegate?
2. If vague, ask the user to clarify
3. Call `spawn_sandbox` with:
   - `agent_id`: a short descriptive name (e.g. "explorer", "builder")
   - `prompt`: clear, specific instructions
4. Tell the user: "Started session **<name>** to work on that. I'll let you know when it's done."
5. After spawning, call `/sessions` to confirm it's running
