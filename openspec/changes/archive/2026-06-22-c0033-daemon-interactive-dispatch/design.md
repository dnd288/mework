## Context

The interactive-session library and event publisher exist and are tested in-process
(`examples/remote-claude/workspace_session_test.go`). The daemon's networked dispatch loop
is one-shot and never opens them. `c0031` now sends an open-session dispatch (with
owner/tenant) and exposes a result + events-ingress endpoint; `c0032` adds the
`session.<id>.input` topic. This change is the runner-side glue.

## Goals / Non-Goals

**Goals:**
- A session-open dispatch opens one long-lived sandbox, keyed by session id.
- Turns from `session.<id>.input` route serially to `Session.Send`.
- Per-turn events reach the server via HTTP for relay to the CLI.
- Duplicate dispatches (SSE resume) are idempotent.

**Non-Goals:**
- Changing the one-shot path (`processDispatch`/`defaultRunAgent`) — it stays for
  non-session dispatches.
- Server-side changes (done in `c0031`/`c0032`).
- CLI commands (`c0034`).

## Decisions

- **`Session != ""` selects the session path.** `dispatchWorker` routes a session-open
  dispatch to `processSessionDispatch`; everything else stays one-shot. No new wire enum.
- **Engine owns a session registry.** `Engine.sessions map[string]*Session` guarded by a
  mutex. On a dispatch whose session id is already open, ack and return without re-opening
  (idempotent under SSE redelivery). This respects one-agent-per-sandbox (a second `Start`
  with the same sandbox id would error).
- **Caller comes from the dispatch.** `Caller{Account: d.Owner, Tenant: d.Tenant, Grant}`
  so `Session.authorizeCaller` matches the session owner/tenant. This is why `c0031` adds
  owner/tenant to the wire.
- **`HTTPDefinitionResolver` for the server-routed path.** Resolve `name@version` from the
  catalog; the published `local-claude@1.0.0` (engine local, backend claude) needs no
  schema change. `FileDefinitionResolver` stays a documented local fallback.
- **`httpBroker` for events egress.** The interactive session's `EventPublisher` needs a
  `bus.Broker`; on the daemon, implement a tiny adapter whose `Publish` POSTs the
  `ChatEvent` payload to the server's events-ingress endpoint (runtime-authed). Chosen over
  collapsing a turn into a single `reportResult` so token/message/done survive.
- **Serial input processing.** One goroutine per session drains `session.<id>.input` and
  calls `Send` sequentially, mirroring the engine's serial `dispatchWorker`. Concurrent
  `Exec` on one sandbox would interleave.
- **Lifecycle.** A close/cancel control message on the input topic maps to
  `Session.Close`/`Cancel`; on close, remove the session from the registry. (Server-side
  idle reap that should tell the daemon to close is a known follow-up — see risks.)

## Risks / Trade-offs

- [Orphaned sandboxes on server reap] → if the server reaps an idle session it does not yet
  tell the daemon to destroy the sandbox; the daemon only tears down on its next call.
  Mitigation: publish a close marker on `session.<id>.input` on close/reap so the daemon's
  input loop calls `Session.Close`. Captured as the cleanup path here.
- [Topic-prefix mismatch] → the Engine subscribes using the raw runner id; the server
  strips a `runner-` prefix when publishing. Covered by a topic-equality test in `c0031`;
  re-assert here for the input topic.
- [Coarse streaming] → the local backend yields one output buffer per turn, so
  `PublishTurn` emits one token + one message + done per whole turn, not fine-grained
  tokens. Acceptable; the CLI must not assume token-level streaming.

## Migration Plan

Additive on the client. The one-shot path is untouched for dispatches without a session id;
the session path activates only for `c0031`'s open-session dispatches.
