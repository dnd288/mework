## 1. Dispatch wire type: owner/tenant (TDD)

- [x] 1.1 Write a test (fail first) asserting `transport.Dispatch` round-trips `Owner`,
      `Tenant`, and `Session` through JSON.
- [x] 1.2 Add `Owner string` and `Tenant string` to `transport.Dispatch`
      (`libs/shared/transport/agent.go`) — additive, JSON-tagged.

## 2. Dispatch helper (TDD)

- [x] 2.1 Write a test (fail first) for `catalog.DispatchSessionToRunner`: it publishes a
      `Dispatch` with the given `Session`, `Owner`, `Tenant`, and a `OpPullAgent|OpSpawn`
      grant to `runner.<id>.dispatch`; assert the topic matches the daemon `Engine`'s
      subscription topic for the same runner id.
- [x] 2.2 Implement `DispatchSessionToRunner(ctx, agentName, runnerID, sessionID, owner,
      tenant string, g *grant.Grant)` in `libs/server/catalog` (sets `msg.Session/Owner/
      Tenant`, reuses the existing publish path).

## 3. Session lifecycle handlers + dispatch trigger (TDD)

- [x] 3.1 Write `handlers_test.go` cases (fail first): `POST /sessions` → 201 +
      `SessionInfo`, owner/tenant taken from the auth context, the injected dispatcher
      called once with the new session id; `GET /sessions` tenant-scoped; `GET
      /sessions/{id}`; `DELETE /sessions/{id}` closes.
- [x] 3.2 Extend `session.Handlers` to hold a dispatcher; `NewHandlers(manager, dispatch)`.
- [x] 3.3 `CreateSession`: after `manager.Create`, build the `OpPullAgent|OpSpawn` grant and
      call `DispatchSessionToRunner` with the session id, owner, tenant.

## 4. Result endpoint (TDD)

- [x] 4.1 Write a test (fail first): `POST /api/v1/runners/sessions/{id}/result` with a
      runtime-auth context and body `{status, summary, error}` → 204.
- [x] 4.2 Implement the result handler (in `session` or `orchestrator`); minimal sink,
      optional terminal `ChatEvent` publish to `session.<id>.control`.

## 5. Mount routes

- [x] 5.1 `router.go`: in the PAT block, mount `POST/GET /sessions`,
      `GET/DELETE /sessions/{id}` via `session.NewHandlers(sessionMgr, dispatcher)`.
- [x] 5.2 `router.go`: under `runtimeAuth`, mount
      `POST /api/v1/runners/sessions/{id}/result`.

## 6. Validation

- [x] 6.1 `make vet` and `make test ./libs/server/...` green; new tests fail-first then
      pass.
- [x] 6.2 `make test` (full) green.
- [ ] 6.3 Smoke: `POST /api/v1/sessions` (PAT) returns a session id and publishes a dispatch
      on the runner topic; the daemon (once `c0033` lands) opens a sandbox.
