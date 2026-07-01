## Why

With `c0031`, a session can be created over HTTP and an open-session dispatch reaches a
runner. To actually **chat**, two more paths are needed and neither exists today:

- **A turn going in** (CLI → server → runner). There is no HTTP route to submit a chat
  turn and no bus topic to carry it to the runner. The existing `session.<id>.control`
  topic is already used by the daemon's `EventPublisher` to publish **outgoing**
  `ChatEvent`s (`libs/client/runner/session_events.go`), so overloading it with incoming
  turns would make the daemon hear its own events.
- **Events streaming back** (runner → server → CLI). The server `session.Handlers.
  AttachSession` returns a control-topic name but is not mounted, and there is no
  CLI-friendly streaming endpoint. There is also no endpoint for the daemon to deliver its
  per-turn `ChatEvent`s to the server (the daemon has no in-process server broker).

This change defines the **direction split** on the bus and the **HTTP transport** for
chat, keeping the server a thin relay (no server-side conversation state). `c0033` makes
the daemon consume the input topic and publish events; `c0034` adds the CLI commands.

## What Changes

- **New per-session input topic.** Add `TopicSessionInput = "session.%s.input"`
  (`libs/server/bus/topics.go`). Direction model: `session.<id>.input` carries **hub →
  runner** turns and control (cancel/close); `session.<id>.control` carries **runner →
  hub** outgoing events (`token`/`message`/`done`/`error`). One direction per topic so the
  runner never receives its own events.
- **Submit a turn (PAT-authed).** `POST /api/v1/sessions/{id}/messages` — verify the
  caller owns the session, then publish the `ChatMessage` to `session.<id>.input`; return
  202 Accepted. The server stores no conversation state — it is a bus relay.
- **Stream events (PAT-authed).** `GET /api/v1/sessions/{id}/stream` — after an ownership
  check, subscribe to `session.<id>.control` and stream `ChatEvent` frames as SSE,
  delegating to the existing `bus.SSEHandler` machinery (heartbeat, resume, bounded
  backpressure). Hides topic naming from the CLI.
- **Events ingress from the runner (runtime-authed).**
  `POST /api/v1/runners/sessions/{id}/events` — accept a marshaled `ChatEvent` from the
  daemon and republish it onto `session.<id>.control`. This is the server endpoint behind
  the daemon's `httpBroker` (added in `c0033`), preserving per-turn token/message/done
  granularity instead of collapsing to a single terminal result.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `message-bus`: clarifies the **session control channel** into a two-topic direction
  model — adds `session.<id>.input` (hub → runner turns/control) and reserves
  `session.<id>.control` for runner → hub outgoing events.
- `prebuilt-agent-sandbox`: adds **server-routed chat transport** — submit a turn, stream
  events, and a runner events-ingress relay — with the server holding no conversation
  state (thin bus relay).

## Impact

- **Server code:** `libs/server/bus/topics.go` (new topic); `libs/server/session/
  handlers.go` (`SendMessage`, `StreamSession`, events-ingress handler reusing
  `bus.SSEHandler`); `libs/server/hub/router.go` (mount send/stream under PAT, events
  ingress under `runtimeAuth`).
- **Reuses** `bus.SSEHandler` (`sse_handler.go`), the `session.ChatMessage`/`ChatEvent`
  DTOs (`libs/server/session/conversation.go`), and the per-subscriber backpressure
  already specified in `message-bus`.
- **Decision: no server-side `Conversation` implementation.** Conversation state and the
  sandbox live on the runner (`runner.Session`); the server relays. `conversation.go`
  stays as the shared DTO source.
- **Depends on** `c0031` (sessions exist + are owned/tenant-scoped). **Precedes** `c0033`
  (daemon consumes `.input`, publishes to ingress) and `c0034` (CLI send/attach).
- No schema migration. Backpressure can drop frames under load — `done`/`error` delivery
  is best-effort; the `c0031` result endpoint remains the authoritative terminal signal.
