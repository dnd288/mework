## Why

Sessions (`c0009-sessions`) give an operator a durable, attachable workspace bound
to a running agent, but attaching only exposes one-shot dispatch and live status —
there is no way to **talk** to the agent interactively. Real work is iterative: the
operator wants to send a message, watch the assistant's reply stream back token by
token, follow up referring to the prior turn, steer with a system prompt, and
cancel a turn that is going the wrong way — all without tearing down the session.

This change introduces **interactive, multi-turn chat** inside a session: a
`Conversation` abstraction that sends messages and streams assistant responses over
the message bus, preserving per-session history.

## What Changes

- A new **chat** capability rooted at `server/session`: a `Conversation` with
  `Send` / `Stream` / `History` / `Cancel`.
- A message turn is modelled as a `ChatMessage` (`Role` ∈ `user` | `assistant` |
  `system` + `Content`); an assistant turn streams back as `ChatEvent`s
  (`Kind` ∈ `token` | `message` | `done` | `error`).
- The assistant response **streams over the message bus** (`c0002-message-bus`)
  rather than a bespoke transport, so chat reuses the existing topic/SSE delivery.
- Conversations are **isolated per session** with per-conversation backpressure: a
  slow chat client buffers/blocks only its own stream and never stalls the agent or
  other sessions.

## Capabilities

### New Capabilities
- `chat`: interactive, multi-turn conversation with a running agent inside a
  session — `Conversation` (`Send`/`Stream`/`History`/`Cancel`), `ChatMessage` /
  `ChatEvent` / `Role`, assistant responses streamed over the message bus with
  per-conversation isolation and backpressure.

## Impact

- **Depends on `c0009-sessions`**: a `Conversation` is opened against an attached
  session bound to a running agent.
- **Depends on `c0002-message-bus`**: assistant turns are **streamed** over the
  bus's topic/SSE delivery; chat does not introduce a new transport.
- Module home: `server/session` (the `Conversation` type and its plumbing).
- Behaviors covered: send-and-stream (CHAT-01), multi-turn history preserved
  (CHAT-02), cancel an in-flight turn keeps the session usable (CHAT-03),
  concurrent chats in different sessions isolated (CHAT-04), a leading system
  prompt steers the turn (CHAT-05), and slow-client backpressure that does not
  stall other sessions (CHAT-06).
