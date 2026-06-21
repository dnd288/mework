## Why

A dispatch today is fire-and-forget: a runner pulls an agent, runs it, reports a
terminal result, and the association disappears. There is no first-class handle on
a **live agent association** — no way to look it up, list it for an operator,
attach to its live wire endpoint, resume it after a reconnect, or close it on
demand. Interactive chat, live status, and online-backed workspaces all need a
durable object to hang off. This change makes the **session** that object: one
session = one live agent association/run, with an explicit lifecycle.

## What Changes

- A new **sessions** capability owning the session lifecycle, with its module home
  in `server/session`.
- A `SessionManager` interface (`Create`/`Get`/`List`/`Attach`/`Close`) defined in
  `shared` alongside the management-view `SessionInfo` and `SessionStatus` types.
- `Create` turns a dispatch into a tracked session; `Attach` returns the **live
  wire endpoint** (the bus `Session` primitive) for chat/stream; `Close`
  terminates the session and tears down its sandbox.
- A management view: `List` enumerates a tenant's sessions with each entry's
  `status` (active|idle|closed) and `owner`.
- **Idle reaping**: a session with no activity past its idle timeout transitions to
  `closed` and its sandbox is destroyed.
- **Ownership and tenant scoping**: only the owner may attach; listings are
  tenant-scoped with no cross-tenant visibility.
- Depends on `c0002-message-bus` (the live `Session` wire primitive that `Attach`
  returns) and `c0005-agent-runner` (the dispatch a session is created from).

## Capabilities

### New Capabilities

- `sessions`: first-class session lifecycle management — create a session from a
  dispatch, get/list sessions (status + owner) per tenant, attach to the live wire
  endpoint, resume after reconnect, enforce ownership, reap idle sessions, and
  close on demand.

## Impact

- **Sequenced after `c0002-repo-restructure`**: session code lands in
  `server/session`; the `SessionManager`, `SessionInfo`, and `SessionStatus`
  contract lives in `shared`.
- Depends on `c0002-message-bus` (the live `Session` wire endpoint `Attach`
  returns) and `c0005-agent-runner` (the `Dispatch` a session is created from).
- Provides the base for `c0010-session-workspaces` (a session attaches an
  online-backed workspace) and `c0011-chat` (an interactive conversation runs
  inside a session).
- Behaviors are pinned by the e2e scenarios `SESSION-01` through `SESSION-07`
  (`tests/e2e/16_sessions_test.go`), driving the `SessionManager`/`SessionInfo`
  surface in `tests/e2e/api_test.go`: lifecycle create→attach→close (SESSION-01),
  list with status+owner (SESSION-02), resume after reconnect (SESSION-03),
  multiple isolated sessions per runner (SESSION-04), idle-timeout reaping
  (SESSION-05), ownership enforced on attach (SESSION-06), and tenant isolation of
  listings (SESSION-07).
