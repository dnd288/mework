## Context

`prebuilt-agent-sandbox` specifies the interactive session lifecycle and the
gateway/registry boundary, and the server already has a `session.Manager` + `session.
Handlers`. The missing wiring is: (1) mounting the session routes, (2) making create
dispatch to a runner, (3) the result endpoint the daemon already POSTs to, and (4) putting
owner/tenant on the dispatch so the runner can authorize. This change is the server half;
`c0032` adds chat transport and `c0033` makes the daemon open the sandbox.

## Goals / Non-Goals

**Goals:**
- A PAT-authed HTTP surface to create/list/get/close a session, tenant-scoped to the
  caller.
- Session-create dispatches an open-session message to the named runner with a pull+spawn
  grant and the owner/tenant.
- A runtime-authed result endpoint matching the daemon's existing POST.

**Non-Goals:**
- Chat turn ingress / event streaming (that is `c0032`).
- The daemon-side handling of the open-session dispatch (that is `c0033`).
- Persisting sessions to Postgres (the manager stays in-memory; revisit if durability is
  needed). Worker auto-selection / channel binding (that remains the webhook path).

## Decisions

- **Reuse `session.Manager`; inject a dispatcher into `Handlers`.** `NewHandlers` gains a
  dispatch dependency. `CreateSession` calls `manager.Create(...)` (already subscribes to
  `session.<id>.control`), then dispatches. Keep owner/tenant sourced from
  `auth.GetAccountID`/`auth.GetTenantID` (the handlers already read these).
- **Dispatch directly, not via the provisioner.** The CLI names the target runner, so
  call a new `catalog.DispatchSessionToRunner` that publishes to `runner.<id>.dispatch`
  with `Dispatch{Session, Owner, Tenant, Grant, ...}`. `AutoProvisioner.Provision` is for
  the webhook path (it also selects a worker and binds a channel) and is intentionally not
  reused, keeping the human-initiated path distinct.
- **`Session != ""` is the open-session signal** (no new enum on the wire). Add only
  `Owner`/`Tenant` to `transport.Dispatch`. The daemon (`c0033`) branches on a non-empty
  `Session`.
- **Result endpoint is a thin sink.** Decode `{status, summary, error}` (matching
  `dispatch.go`), 204 on success. It exists so the daemon's terminal POST (and any failed
  session open) does not 404; it MAY publish a terminal `ChatEvent` to the control topic.
  Mount it under `runtimeAuth` (same authenticator as `/api/v1/jobs/subscribe`).
- **Auth split is load-bearing.** Human session CRUD under the existing
  `r.Use(patAuth.Middleware)` block; the runner result endpoint under `runtimeAuth`. Mixing
  these would 401 one side.

## Risks / Trade-offs

- [Ownership propagation] → without `Owner`/`Tenant` on the dispatch, the daemon's
  `Session.authorizeCaller` rejects turns. These fields are the minimal addition that makes
  the runner able to authorize.
- [Topic-prefix consistency] → the server builds the dispatch topic from the runner id;
  ensure it matches the daemon's `Engine` subscription (`engine.go:60`). Add a test
  asserting both produce the same topic (`DispatchToRunner` strips a `runner-` prefix via
  `runnerShortName`, the Engine uses the raw id).
- [In-memory sessions] → a server restart drops session metadata; acceptable for the
  interactive workflow now, called out as future durability work.

## Migration Plan

Additive. New routes and a new dispatch helper; `transport.Dispatch` gains two optional
fields (zero-valued for existing one-shot dispatches, so the daemon's current path is
unaffected until `c0033`).
