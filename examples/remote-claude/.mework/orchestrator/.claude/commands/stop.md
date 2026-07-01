---
name: "stop"
description: "Stop and clean up a work session"
category: Orchestrator
tags: [stop, cancel, destroy, session, sandbox]
---

Stop and remove a session. Usage:

`/stop <session-id>` or `/cancel <session-id>`

1. Confirm with the user before destroying
2. Call `destroy_sandbox` with the sandbox ID
3. Confirm it's been cleaned up
