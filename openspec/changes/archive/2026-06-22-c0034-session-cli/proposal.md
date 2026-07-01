## Why

The server now exposes the session lifecycle (`c0031`) and chat transport (`c0032`), and
the daemon drives the interactive sandbox (`c0033`). The last missing piece for a developer
to **drive the workflow from the terminal** is the CLI. Today `mework session list`
(`libs/client/cli/cmd_session.go`) is a **stub** that always prints an empty list and never
contacts the server, and there is **no `session create`, `send`, `attach`, or `close`**
command. Without these, the only way to exercise server-routed chat is by hand-rolling
HTTP/SSE calls.

This change makes `mework session` a real, PAT-authed client of the session API so the
full manual test reads: enroll → daemon start → `session create` → `session attach` +
`session send` → `session close`.

## What Changes

- **`session list`** — replace the stub with a real `GET /api/v1/sessions`, rendering the
  table (and `--json`). Tenant scope is the server's (derived from the PAT caller).
- **`session create --agent <name> [--runner <id>] [--version <v>] [--json]`** —
  `POST /api/v1/sessions`; print the new session id.
- **`session send <id> <message>`** — `POST /api/v1/sessions/{id}/messages` with
  `{role:"user", content}`; 202 on accept.
- **`session attach <id>`** — open the SSE stream `GET /api/v1/sessions/{id}/stream`, decode
  each `ChatEvent` frame, print `token`/`message` content, and stop on `done`/`error`. Use
  a client-side **idle timeout** so a dropped terminal does not hang the terminal forever.
- **`session close <id>`** — `DELETE /api/v1/sessions/{id}`.
- All session commands authenticate with the **PAT** (loaded from the existing login/auth
  config), not the runner secret, and are registered under the existing `sessionCmd`
  (already wired into help, `libs/client/cli/help.go:42`).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cli`: the **Command surface** requirement's session commands become a real client of the
  server session API — `session list` queries the server, and `create`/`send`/`attach`/
  `close` are added to drive a server-routed interactive chat from the terminal.

## Impact

- **Client code:** `libs/client/cli/cmd_session.go` (real `list`; add `create`, `send`,
  `attach`, `close`; register in `init()`). New test `cmd_session_test.go`.
- **Reuses** the SSE reader from `libs/client/subscribe` if it can target an arbitrary path,
  else a small SSE decode loop; the PAT config loader used by other management commands.
- **Auth:** PAT (human caller) — the same token the existing `runtime`/`profile`/`provider`
  management commands use.
- **Depends on** `c0031` (lifecycle routes) and `c0032` (send/stream routes). Independent of
  `c0033` to build, but a useful end-to-end demo requires `c0033` (a daemon that opens the
  sandbox).
- No server change, no schema migration.
