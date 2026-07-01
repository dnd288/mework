## Context

`c0031` gives sessions an HTTP lifecycle and an open-session dispatch. Chat needs a turn-in
path and an events-out path. The daemon already publishes `ChatEvent`s to
`session.<id>.control`; the server `Manager` already subscribes to that topic. What is
missing is the input topic, the HTTP send/stream endpoints, and an ingress endpoint so the
daemon (which has no in-process server broker) can deliver events to the server bus.

## Goals / Non-Goals

**Goals:**
- A clear, single-direction-per-topic bus model for sessions.
- HTTP: submit a turn, stream events back, and accept runner events.
- Keep the server a thin relay (no conversation state).

**Non-Goals:**
- Daemon-side consumption of `.input` / publishing of events (that is `c0033`).
- CLI commands (that is `c0034`).
- Implementing the `Conversation` interface server-side.

## Decisions

- **Two topics, one direction each.** `session.<id>.input` = hub → runner (turns + control
  like cancel/close); `session.<id>.control` = runner → hub (outgoing `ChatEvent`s). This
  prevents the daemon from receiving its own published events and matches the existing
  `EventPublisher` direction on `.control`.
- **Server is a bus relay, not a conversation owner.** `SendMessage` publishes the
  `ChatMessage` to `.input` and returns 202; it records nothing. `StreamSession` subscribes
  to `.control` and relays. The runner owns conversation state via `runner.Session`. This
  is the KISS/YAGNI choice and matches the existing webhook relay.
- **Reuse `bus.SSEHandler` for streaming.** `StreamSession` enforces ownership, then
  delegates to the existing SSE handler subscribing to `session.<id>.control` — inheriting
  heartbeat, last-event-id resume, and bounded backpressure rather than hand-rolling an SSE
  loop. (For a PAT caller the handler's runtime-id tag is absent and defaults to a
  placeholder, which is fine for identity tagging only.)
- **Events ingress republishes.** `POST /api/v1/runners/sessions/{id}/events` (runtime
  auth) takes a raw `ChatEvent` JSON and `broker.Publish`es it to `.control`. This is the
  server half of the daemon's `httpBroker` (c0033), chosen over collapsing a turn into one
  terminal result so token/message granularity survives.
- **Auth split.** send/stream are PAT (human); events ingress is runtime (`rt_`). Ownership
  is checked against the session owner from the manager before publish/subscribe.

## Risks / Trade-offs

- [Backpressure drops frames] → `SSEHandler` drops oldest under load; a `done`/`error`
  could be dropped. Treat the `c0031` result endpoint as the authoritative terminal; the
  CLI (`c0034`) uses an idle timeout. Documented, not engineered away here.
- [Topic overload regression] → keeping `.control` strictly outbound is essential; a test
  asserts a turn published to `.input` is not delivered to a `.control` subscriber.
- [Spec divergence] → the prior `message-bus` requirement described `.control` as carrying
  hub→sandbox input; this change re-states the direction model so spec and code agree.

## Migration Plan

Additive: one new topic constant and three new routes. Existing `.control` publishers/
subscribers are unchanged in direction (runner publishes, hub subscribes); only the
incoming-turn path is new.
