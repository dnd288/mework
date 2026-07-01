## Why

`daemon-runtime` already specifies that the daemon can drive a **long-lived sandbox per
session** and **stream per-turn events**, and the library implements exactly that:
`OpenSession`/`Send`/`Cancel`/`Close` (`libs/client/runner/interactive_session.go`) and
`EventPublisher.PublishTurn` (`libs/client/runner/session_events.go`). **But nothing on the
network path invokes it.** Two disconnects:

- The daemon's dispatch loop is **one-shot only**. `Engine.dispatchWorker` →
  `processDispatch` → `defaultRunAgent` (`libs/client/runner/dispatch.go:119`) pulls the
  agent, starts a sandbox, execs once over stdin, destroys it, reports, and acks. It never
  calls `OpenSession`, so a session-open dispatch from `c0031` has nowhere to go and chat
  turns from `c0032`'s `session.<id>.input` are never consumed.
- The interactive session's `EventPublisher` writes `ChatEvent`s to a `bus.Broker`, but on
  the daemon there is **no in-process server broker** — events must travel to the server
  over HTTP.

This change connects them: an open-session dispatch (non-empty `Session`) opens a
long-lived interactive session keyed by id; the daemon subscribes to `session.<id>.input`
and routes each turn serially to `Session.Send`; and a small `httpBroker` delivers the
session's `ChatEvent`s to the server's events-ingress endpoint (`c0032`) for relay to the
CLI. This is the runner half of the server-routed interactive workflow.

## What Changes

- **Branch the dispatch loop on a session-open dispatch.** In `Engine` (`engine.go`), a
  dispatch with a non-empty `Session` is routed to a new `processSessionDispatch` instead
  of the one-shot `processDispatch`. The engine keeps a mutex-guarded
  `sessions map[string]*Session` so a duplicate dispatch (SSE resume/redelivery) does
  **not** re-`OpenSession`.
- **Open a long-lived interactive session.** `processSessionDispatch`
  (new `libs/client/runner/session_dispatch.go`) verifies the grant (enforces `OpSpawn`),
  builds a `Caller{Account: d.Owner, Tenant: d.Tenant, Grant}` from the dispatch (the
  owner/tenant `c0031` puts on the wire), resolves the definition over the catalog
  (`HTTPDefinitionResolver`), and calls `OpenSession` once — storing the `*Session` by id.
- **Consume turns from the input topic.** The daemon subscribes to `session.<id>.input`,
  decodes each `ChatMessage`, and routes it to `Session.Send` **serially** per session
  (one goroutine), preserving the one-agent-per-sandbox invariant. A close/cancel control
  message ends the session (`Session.Close`/`Cancel`).
- **Deliver events to the server.** A new `httpBroker` (client) implements `bus.Broker`'s
  `Publish` by POSTing the marshaled `ChatEvent` to
  `POST /api/v1/runners/sessions/{id}/events` (runtime-authed) so per-turn token/message/
  done granularity reaches the CLI via the server relay.
- **Definition source.** Use `HTTPDefinitionResolver` (server-routed; the published
  `local-claude@1.0.0` definition). The existing `mework.yml` schema (`engine: local`,
  `backend: claude`) is sufficient — no schema change. `FileDefinitionResolver` remains a
  documented local fallback.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `daemon-runtime`: adds that an **open-session dispatch drives the interactive session** —
  the daemon opens a long-lived sandbox on a session-open dispatch, consumes turns from the
  session input topic and routes them to the session, guards duplicate dispatches, and
  delivers per-turn events to the server for relay.

## Impact

- **Client code:** `libs/client/runner/engine.go` (session registry + branch in
  `dispatchWorker`); new `libs/client/runner/session_dispatch.go` (`processSessionDispatch`
  + input subscription loop); new `httpBroker` adapter (events egress).
- **Reuses** `OpenSession`/`Send`/`Close` (`interactive_session.go`), `EventPublisher`
  (`session_events.go`), the SSE subscribe client (`libs/client/subscribe`), and
  `HTTPDefinitionResolver` (`libs/client/catalog`).
- **Depends on** `c0031` (open-session dispatch with owner/tenant + result endpoint) and
  `c0032` (`session.<id>.input` topic + events-ingress endpoint). **Precedes** `c0034`
  (CLI) only in usefulness — they are independent to implement.
- Preserves stdin-not-argv, one-agent-per-sandbox, and the c0027 boundary (sandbox runs on
  the runner). No server change here. No schema migration.
