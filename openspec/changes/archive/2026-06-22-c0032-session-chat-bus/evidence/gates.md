# Evidence — c0032-session-chat-bus

Date: 2026-06-22
Branch: feat/c0032-session-chat-bus
Toolchain: go1.26.4 (go.mod requires >= 1.25)

## Gates

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | PASS |
| Vet | `make vet` | PASS |
| Test (full) | `go test -p 1 ./...` | PASS (exit 0) |
| OpenSpec validate | `openspec validate c0032-session-chat-bus --strict` | PASS ("is valid") |

DB-backed tests (`TEST_DATABASE_URL` unset) skip, including the new
`TestSessionChatRoutes_Mounted` router test — expected in this environment.

## What was implemented (test-first, Red→Green→commit per unit)

1. **Input topic + direction model** — added `bus.TopicSessionInput`
   (`session.%s.input`, hub → runner) alongside the existing
   `session.%s.control` (runner → hub). Test asserts a turn published to
   `.input` reaches an `.input` subscriber but not a `.control` subscriber nor
   another session's `.input` (no cross-direction, no cross-session leakage).
2. **SendMessage handler** — decodes a `ChatMessage`, checks the caller owns the
   session (`manager.Get` owner vs `auth.GetAccountID`), publishes to
   `session.<id>.input`, returns 202. Non-owner → 403, no publish.
3. **StreamSession handler** — ownership check, then delegates to
   `bus.SSEHandler` targeting `session.<id>.control` (inherits heartbeat /
   resume / bounded backpressure). Non-owner → 403 before subscribing.
4. **Runner events ingress + routes** — `ReceiveEvents` republishes a raw
   `ChatEvent` JSON to `session.<id>.control`. Routes mounted in `hub/router.go`:
   `POST /api/v1/sessions/{id}/messages` and `GET /api/v1/sessions/{id}/stream`
   under the PAT block; `POST /api/v1/runners/sessions/{id}/events` under
   `runtimeAuth`.

## Coverage (new code)

- `bus.FormatTopic` / `TopicSessionInput`: 100%
- `session.SendMessage`: 60%, `StreamSession`: 84.6%, `ReceiveEvents`: 64.7%,
  `ownsSession`: 71.4%

See `coverage.txt` and `test-results.md`.
