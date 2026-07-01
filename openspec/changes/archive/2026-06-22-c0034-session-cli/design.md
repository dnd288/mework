## Context

`c0031`/`c0032` expose the session lifecycle and chat transport over HTTP; `c0033` makes the
daemon open the sandbox. The CLI is the last surface. `cmd_session.go` has a stub `list` and
no other verbs. This change makes the `mework session` group a real PAT-authed client.

## Goals / Non-Goals

**Goals:**
- `session list/create/send/attach/close` working against the server API.
- `attach` streams `ChatEvent`s and terminates cleanly on `done`/`error` or idle.
- PAT auth, consistent with other management commands.

**Non-Goals:**
- Any server or daemon change.
- Rich TUI / multiplexed attach+send in one process (two terminals is fine for the demo).
- Conversation history rendering beyond the live stream.

## Decisions

- **PAT auth for all session verbs.** These are human commands; load the PAT from the
  existing auth/login config (the same source other `/api/v1` management commands use), not
  the runner identity. (The runner secret is for daemon ↔ server only.)
- **`attach` reuses SSE decoding.** Prefer the existing `libs/client/subscribe` SSE client
  if it can target `/api/v1/sessions/{id}/stream`; otherwise a minimal reader that parses
  `data:` frames into `session.ChatEvent` and prints `Content` for `token`/`message`,
  stopping on `done`/`error`.
- **Idle timeout on `attach`.** Because the bus may drop a terminal frame under
  backpressure, `attach` exits after a configurable idle interval with no events, rather
  than blocking forever. (The `c0031` result endpoint is the authoritative terminal
  server-side.)
- **`create` flags.** `--agent` required; `--runner` (target runner id; default the enrolled
  runner if resolvable), `--version` (default `latest`), `--json`. Print the session id so
  it can be captured into a shell variable.
- **Keep the test-friendly command structure.** Mirror the existing pattern where leaf
  commands are addressable for unit tests (as `sessionListCmd` already is), so each verb is
  table-testable against an `httptest` stub.

## Risks / Trade-offs

- [Two-terminal demo] → `attach` (stream) and `send` (turn) run as separate invocations;
  acceptable and matches the documented manual test. A combined REPL is future work.
- [Auth config source] → must match whatever `login`/`auth` persist; if absent, the command
  should error with guidance ("run `mework login` first"), mirroring the existing
  not-enrolled guidance in `session list`.
- [SSE client reuse] → if `subscribe` is too coupled to the jobs path, fall back to a small
  local reader; keep it minimal.

## Migration Plan

Additive CLI surface. `session list` changes from a stub to a real call (same output shape).
New verbs are net-new under the existing `session` group.
