# Gates — c0033-daemon-interactive-dispatch

Toolchain: go 1.26.4 (go.work multi-module workspace; `go test ./...` from the
root only covers the root module, so per-module import paths are used below).

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | PASS (exit 0) |
| Vet | `make vet` | PASS (all modules clean) |
| Tests (client/server/shared) | `go test -p 1 -count=1 mework/libs/client/... mework/libs/server/... mework/libs/shared/...` | PASS — 31 packages ok |
| Tests (sandbox/tests/tools) | `go test -p 1 -count=1 mework/libs/sandbox/... mework/libs/tests/... mework/libs/tools/...` | PASS — 9 packages ok |
| Tests (examples) | `cd examples/remote-claude && go test -p 1 -count=1 ./...` | PASS — 1 package ok |
| Tests (root module) | `go test -p 1 ./...` | PASS (exit 0) |
| Spec validate | `openspec validate c0033-daemon-interactive-dispatch --strict` | PASS — "is valid" |

DB-backed tests (require `TEST_DATABASE_URL`) were not exercised here; the change
touches no DB paths.

## New / changed behavior under test

- `TestEngine_RoutesSessionDispatch` — open-session dispatch routes to the session
  path; duplicate dispatch for an already-open id is idempotent (acked, no re-open).
- `TestEngine_NonSessionDispatchStaysOneShot` — a dispatch without owner/tenant
  stays on the one-shot path.
- `TestProcessSessionDispatch_OpenAndTurns` — one long-lived sandbox opened once;
  two input-topic turns run serially on the same sandbox; per-turn
  token/message/done events egress to the server events endpoint.
- `TestSessionInput_ControlClosesAndCancels` — close control destroys the sandbox
  and removes the session from the registry; cancel interrupts without destroying.
- `TestHTTPBroker_PublishPostsEvent` / `TestHTTPBroker_PublishErrorsOnNon2xx` —
  events POST to `/api/v1/runners/sessions/{id}/events` with the runtime
  credential; non-2xx and non-session topics error.
