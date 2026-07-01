## Context

A session (`c0009-sessions`) binds an operator to a single running agent and
already exposes attach + live status. The message bus (`c0002-message-bus`) already
delivers server→client events as a resumable, topic-addressed SSE stream with
per-client backpressure. Interactive chat sits exactly between these two: it is
session-scoped state (the ordered history of turns) whose assistant output is a
stream of events. Rather than invent a new transport, chat is a thin `Conversation`
abstraction in `server/session` that records turns and publishes the assistant's
reply onto the bus. The behavioral surface is fixed by the `Conversation`
interface: `Send`, `Stream`, `History`, `Cancel`.

## Goals / Non-Goals

**Goals:**
- A `Conversation` per session exposing `Send` / `Stream` / `History` / `Cancel`.
- Multi-turn history preserved in order, including a leading system prompt.
- Assistant responses streamed as `ChatEvent`s over the message bus.
- Strict per-session isolation: no cross-talk between concurrent conversations.
- Per-conversation backpressure: a slow reader never stalls the agent or peers.
- Mid-turn cancel that interrupts generation and leaves the session usable.

**Non-Goals:**
- The session lifecycle itself (attach/detach/bind) — owned by `c0009-sessions`.
- The bus transport (topics, SSE framing, resumption, ack) — owned by
  `c0002-message-bus`; chat is a producer/consumer on top of it.
- Persisting full conversation transcripts beyond the session's retention.
- Multi-agent or multi-user fan-in within a single conversation.

## Decisions

- **`Conversation` is the surface.** `Send(ctx, ChatMessage)` appends a turn and
  triggers an assistant turn; `Stream() <-chan ChatEvent` yields that turn's
  events; `History(ctx) ([]ChatMessage, error)` returns turns in order;
  `Cancel(ctx)` interrupts the in-flight turn.
- **`Role` ∈ `user` | `assistant` | `system`.** A `system` message steers the turn
  and, when present, leads the recorded history (it is the first turn).
- **`ChatEvent` kinds ∈ `token` | `message` | `done` | `error`.** A turn streams
  zero or more `token` (and/or whole-`message`) events and terminates with exactly
  one terminal event: `done` on success or `error` on failure/refusal. Cancel
  surfaces as a prompt stream termination.
- **Stream over the bus.** Each conversation maps to a per-session bus topic; the
  assistant's `ChatEvent`s are published there and delivered to the client via the
  existing SSE delivery, reusing its ordering and resumption — chat adds no new
  wire protocol.
- **Per-conversation isolation.** Conversation state and its event stream are keyed
  by session id; a publish for one session is never delivered on another's stream.
  Concurrent `Send`s on different sessions proceed independently.
- **Per-conversation backpressure.** Each conversation's stream is independently
  buffered; when a client drains slowly, backpressure is contained to that
  conversation (buffer then block/shed for that reader only) so the agent and other
  sessions keep flowing.

## Risks / Trade-offs

- **Cancel race.** A `Cancel` may arrive after the terminal event is already
  enqueued; the contract is "stream stops promptly and the session stays usable",
  so a late-arriving `done`/`error` after cancel is acceptable and the next `Send`
  must still work.
- **Backpressure policy.** Buffer-then-block protects ordering but can stall a slow
  reader's own turn; buffer-then-shed protects liveness but drops tokens. The
  decision is to isolate the policy per conversation so whichever is chosen cannot
  affect other sessions.
- **History growth.** Unbounded history inflates each turn's context; bounded by
  the session's retention window rather than kept forever.
- **Ordering vs. concurrency.** Within one conversation, turns and their events
  must stay ordered; across conversations they are deliberately independent —
  mixing these guarantees is the main source of subtle bugs and is called out so
  tests pin both.
