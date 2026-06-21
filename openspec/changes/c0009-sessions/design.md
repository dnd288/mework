## Context

The agent-hub redesign (`docs/target-architecture.md`) introduces a live
**Session** wire primitive on the bus (`c0002-message-bus`): a control channel
plus `PushToSandbox`. But there is no object that *owns* a session over its
lifetime — nothing to create, look up, list, attach to, or close. Interactive chat
(`c0011-chat`), live status, and online-backed workspaces (`c0010-session-workspaces`)
all need a durable, manageable session object. This change adds that object and its
manager, separate from the bus primitive: the bus `Session` is the live endpoint
`Attach` hands back, while `SessionManager` owns the lifecycle around it.

## Goals / Non-Goals

**Goals:**

- A first-class session = one live agent association/run, with an explicit
  create→attach→close lifecycle.
- A `SessionManager` (`Create`/`Get`/`List`/`Attach`/`Close`) and a management-view
  `SessionInfo` in `shared`, with the implementation home in `server/session`.
- A management listing exposing each session's status and owner, tenant-scoped.
- Resume an attached session after a dropped connection without losing state.
- Idle reaping that closes the session and destroys its sandbox.
- Ownership and tenant isolation enforced on attach and list.

**Non-Goals:**

- The live wire/control-channel internals (`c0002-message-bus` owns the bus
  `Session` primitive and SSE resume).
- The dispatch/grant issuance (`c0005-agent-runner`).
- Interactive chat semantics (`c0011-chat`) and workspace mounting
  (`c0010-session-workspaces`) — sessions are the object those build on.

## Decisions

- **`SessionManager` API.** `Create(d Dispatch) → SessionInfo` turns a dispatch
  into a tracked session; `Get(id) → SessionInfo` and `List(tenant) → []SessionInfo`
  are the management views; `Attach(id) → Session` returns the **live wire
  endpoint** (the bus `Session` primitive) for chat/stream; `Close(id)` terminates
  the session. The manager is distinct from the bus `Session` it returns.
- **`SessionInfo` shape.** A session carries `{id, tenant, runner, agent, status,
  owner, created}` — enough for an operator to manage it without touching the live
  wire. `id` is the `SessionID`; `runner` and `agent` identify what is running;
  `owner` is the `AccountID` that created it; `created` is the creation time.
- **Status is `active | idle | closed`.** `active` while attached/working, `idle`
  with no activity, `closed` once terminated (by `Close` or by idle reaping).
  `closed` is terminal.
- **`Attach` returns the live wire endpoint.** Attaching does not create a new
  association; it hands back the existing live `Session` (control channel +
  push-to-sandbox), so a re-attach after a reconnect resumes the still-running
  agent rather than starting a new one.
- **Idle reaping is part of the lifecycle.** A session past its idle timeout
  transitions to `closed` and its sandbox is destroyed — sessions never leak
  sandboxes.
- **Ownership and tenancy are enforced by the manager.** `Attach` is allowed only
  for the owning account; `List` only ever returns sessions in the requested
  tenant.

## Risks / Trade-offs

- **Idle-timeout tuning.** Too short reaps useful interactive sessions; too long
  leaks sandboxes. The timeout is configurable and reaping is driven by observed
  activity, not wall-clock alone.
- **Reaper vs. in-flight attach race.** A reattach arriving as the reaper fires
  must not resurrect a sandbox already torn down; the transition to `closed` is the
  single source of truth and `Attach` on a closed session fails.
- **Manager/bus boundary.** Keeping `SessionManager` separate from the bus
  `Session` adds an indirection, but it isolates lifecycle/ownership/tenancy from
  the wire transport so each can evolve independently.
- **Ownership model granularity.** Ownership is per-account today; team/shared
  sessions are deferred and would extend, not replace, this model.
