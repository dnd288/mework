## 1. Input topic + direction model (TDD)

- [x] 1.1 Write a test (fail first): a message published to `session.<id>.input` is
      delivered to an `.input` subscriber but NOT to a `session.<id>.control` subscriber
      (no cross-topic / cross-direction leakage).
- [x] 1.2 Add `TopicSessionInput = "session.%s.input"` to `libs/server/bus/topics.go`.

## 2. Submit a turn (TDD)

- [x] 2.1 Write a test (fail first): `POST /api/v1/sessions/{id}/messages` with an owning
      caller publishes the `ChatMessage` to `session.<id>.input` and returns 202; a
      non-owner is denied.
- [x] 2.2 Implement `session.Handlers.SendMessage` (decode `ChatMessage`, ownership check
      via `manager.Get` + `auth.GetAccountID`, `broker.Publish` to `.input`). Add a
      `broker` field to `Handlers`.

## 3. Stream events (TDD)

- [x] 3.1 Write a test (fail first): publish a `ChatEvent` to `session.<id>.control`; a
      `GET /api/v1/sessions/{id}/stream` response contains the event as an SSE `data:`
      frame; a non-owner is denied before subscribing.
- [x] 3.2 Implement `session.Handlers.StreamSession`: ownership check, then delegate to
      `bus.SSEHandler.Subscribe` targeting `session.<id>.control`.

## 4. Events ingress from the runner (TDD)

- [x] 4.1 Write a test (fail first): `POST /api/v1/runners/sessions/{id}/events`
      (runtime-auth) with a `ChatEvent` body republishes it to `session.<id>.control`
      (assert via a `.control` subscriber); unauthenticated is rejected.
- [x] 4.2 Implement the events-ingress handler (republish to `.control`).

## 5. Mount routes

- [x] 5.1 `router.go`: under the PAT block, mount `POST /sessions/{id}/messages` and
      `GET /sessions/{id}/stream`.
- [x] 5.2 `router.go`: under `runtimeAuth`, mount
      `POST /api/v1/runners/sessions/{id}/events`.

## 6. Validation

- [x] 6.1 `make vet` and `make test ./libs/server/...` green; new tests fail-first then
      pass.
- [x] 6.2 `make test` (full) green.
