---
name: "status"
description: "Check the status of a specific work session"
category: Orchestrator
tags: [status, check, session, sandbox]
---

Check a specific session's status. Usage:

`/status <session-id>` or `/status <session-name>`

1. Call `get_sandbox_status` with the sandbox ID
2. If the session is done, report its output and offer to clean up
3. If still running, report progress estimate
4. If failed, report the error and offer to retry or adjust
