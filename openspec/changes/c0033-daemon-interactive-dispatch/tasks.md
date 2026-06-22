## 1. Engine session registry + branch (TDD)

- [x] 1.1 Write a test (fail first): a dispatch with a non-empty `Session` is routed to the
      session path (not one-shot), and a second dispatch with the same session id does NOT
      re-open (idempotent).
- [x] 1.2 Add `sessions map[string]*Session` (+ mutex) to `Engine`; branch in
      `dispatchWorker` on `d.Session != ""` to `processSessionDispatch`.

## 2. processSessionDispatch + input loop (TDD)

- [x] 2.1 Write `session_dispatch_test.go` (fail first): a fake `DefinitionResolver`
      returning a local-engine stub backend (mirror `workspace_start_test.go` /
      `stub-backend.sh`), a fake broker capturing publishes. Drive one open + two input
      turns: assert `OpenSession` started once, both turns ran on the same sandbox, and
      `token`/`message`/`done` were published per turn.
- [x] 2.2 Implement `processSessionDispatch` (`libs/client/runner/session_dispatch.go`):
      verify grant (enforce `OpSpawn`), build `Caller{Account:d.Owner, Tenant:d.Tenant,
      Grant}`, deps with `HTTPDefinitionResolver` + `httpBroker`, `OpenSession` once, store
      by id, ack the dispatch.
- [x] 2.3 Subscribe to `session.<id>.input`; per message, route to `Session.Send` serially
      (one goroutine per session); a close/cancel control message maps to
      `Session.Close`/`Cancel` and removes the session from the registry.

## 3. httpBroker events egress (TDD)

- [x] 3.1 Write a test (fail first): the `httpBroker.Publish` POSTs the `ChatEvent` payload
      to `POST /api/v1/runners/sessions/{id}/events` with the runtime credential (assert
      against an `httptest` stub).
- [x] 3.2 Implement `httpBroker` (client) implementing `bus.Broker.Publish`; pass it to the
      session deps so `EventPublisher` delivers events to the server.

## 4. Definition fixture (no schema change)

- [x] 4.1 Confirm/author a `local-claude@1.0.0` definition (engine: local, backend: claude)
      publishable via the existing catalog publish path; document the server-routed
      resolve. Note `FileDefinitionResolver` as the local fallback.

## 5. Validation

- [ ] 5.1 `make vet` and `make test ./libs/client/runner/...` green; new tests fail-first
      then pass.
- [ ] 5.2 `make test` (full) green.
- [ ] 5.3 Smoke (with `c0031`/`c0032`): create a session → daemon opens a local/claude
      sandbox; a turn on `.input` runs and events appear on `.control`.
