## Why

The `prebuilt-agent-sandbox` capability already specifies an interactive session with an
explicit **create / attach / close** lifecycle, owned and tenant-scoped, with the c0027
boundary (server = gateway + registry; the runner executes). The server-side pieces exist
in code — `session.Manager` (`libs/server/session/session.go`) creates/lists/closes
sessions and subscribes to the control topic, and `session.Handlers`
(`libs/server/session/handlers.go`) implements `CreateSession`/`GetSession`/
`ListSessions`/`AttachSession`/`CloseSession`. **But none of it is reachable**:

- `libs/server/hub/router.go` mounts **no `/api/v1/sessions` routes** (the handlers are
  never registered), so a client cannot create or manage a session over HTTP.
- Creating a session does **not dispatch** anything to a runner, so even if it were
  reachable, no sandbox would open on the client.
- The client daemon's `reportResult` (`libs/client/runner/dispatch.go:176`) POSTs a
  terminal result to `POST /api/v1/runners/sessions/{id}/result`, which **has no server
  route** — it 404s today.
- The dispatch wire type (`transport.Dispatch`) carries no **owner/tenant**, so a runner
  receiving a session-open dispatch cannot satisfy the daemon's owner/tenant authorization
  check (`interactive_session.go:222`).

This change wires the server session **lifecycle over HTTP** and makes session-create
**dispatch an open-session message to a named runner**, plus the result sink the daemon
already targets. It is the server half of the server-routed interactive workflow; the
daemon half (turning that dispatch into a long-lived sandbox) is `c0033`, and chat
turn/stream transport is `c0032`.

## What Changes

- **Session lifecycle HTTP API (PAT-authed).** Mount, in the PAT-guarded `/api/v1` block
  of `router.go`: `POST /sessions` (create), `GET /sessions` (list, tenant-scoped),
  `GET /sessions/{id}` (get), `DELETE /sessions/{id}` (close). Owner/tenant are derived
  from the authenticated caller, never from request args (per the existing
  remote-control-authorization requirement).
- **Create triggers a dispatch to the named runner.** `CreateSession` mints an
  `OpPullAgent | OpSpawn` grant (as the channel provisioner already does,
  `provisioner.go:70`) and dispatches an **open-session** message to the request's runner.
  A new `catalog.DispatchSessionToRunner(ctx, agent, runner, sessionID, owner, tenant,
  grant)` publishes a `Dispatch` carrying the session id, owner, and tenant on
  `runner.<id>.dispatch`. It calls dispatch **directly** (the CLI names the runner) — it
  does **not** reuse `AutoProvisioner.Provision`, which also does `SelectWorker` + channel
  `Bind` (the webhook path).
- **Dispatch carries owner/tenant; non-empty `Session` marks an open-session dispatch.**
  Add `Owner` and `Tenant` fields to `transport.Dispatch` (`libs/shared/transport`). The
  existing `Session` field, when non-empty, is the signal the daemon uses (in `c0033`) to
  open a long-lived session rather than run a one-shot agent.
- **Result endpoint (runtime-authed).** Mount `POST /api/v1/runners/sessions/{id}/result`
  under the runtime (`rt_`) authenticator so the daemon's `reportResult` has a real
  endpoint. Minimal status sink: record/log and return 204; optionally publish a terminal
  `ChatEvent` on the session control topic.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: adds the **server session lifecycle HTTP API** and makes
  **session-create dispatch an open-session message to a named runner**, plus the
  **runner result endpoint**. The dispatch wire type gains owner/tenant so the runner can
  authorize turns. Stays within the c0027 boundary (the server still never spawns a
  sandbox; it only stores session metadata, dispatches, and relays).

## Impact

- **Server code:** `libs/server/hub/router.go` (mount session + result routes);
  `libs/server/session/handlers.go` (inject a dispatcher; `CreateSession` builds the grant
  and dispatches; add a result handler); `libs/server/catalog` (new
  `DispatchSessionToRunner`).
- **Shared:** `libs/shared/transport` — add `Owner`, `Tenant` to `Dispatch` (additive).
- **Auth split:** session CRUD is **PAT**-authed (human caller); the result endpoint is
  **runtime**-authed (the daemon sends `Authorization: Bearer <secret>`).
- **Depends on** `c0030` (an enrolled runner to dispatch to). **Precedes** `c0032`
  (chat transport) and `c0033` (daemon opens the session).
- No schema migration (sessions are in-memory in the manager today; unchanged here).
  stdin-not-argv and the c0027 server boundary are preserved.
