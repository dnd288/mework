# Verification Gates — c0031-interactive-session-api

Toolchain: Go 1.26.4 (go.mod requires >= 1.25). Tests run with `-p 1`.
DB-backed tests run against `TEST_DATABASE_URL` (local Postgres 16, `make test-db`).

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` (root) + per-module via `make` | PASS |
| Vet | `make vet` | PASS (all modules clean) |
| Touched-package tests | `go test ./catalog/ ./session/ ./hub/` (libs/server), `./transport/` (libs/shared), `./runner/` (libs/client) | PASS |
| Full suite | `make test` (all workspace modules, `-p 1`) | PASS except 4 pre-existing failures unrelated to this change (see below) |
| OpenSpec validate | `openspec validate c0031-interactive-session-api --strict` | PASS ("is valid") |

## New tests (Red -> Green per unit)

- Unit 1 — `libs/shared/transport/dispatch_test.go`: `TestDispatch_OwnerTenantRoundTrip` — Dispatch JSON round-trips Owner/Tenant/Session.
- Unit 2 — `libs/server/catalog/session_dispatch_test.go`: `TestDispatchSessionToRunner` (+ `_NilGrant`) — publishes session+owner+tenant+grant on the runner dispatch topic; asserts the topic equals `runner.DispatchTopic(runnerID)` (the daemon Engine's subscription topic).
- Unit 3 — `libs/server/session/handlers_test.go`: `TestCreateSession_DispatchesAndUsesAuthContext`, `TestListSessions_TenantScoped`, `TestGetAndCloseSession`, `TestResultSession_204` — handlers source owner/tenant from auth context, call the injected dispatcher once, list is tenant-scoped, result endpoint returns 204.
- Unit 4 — `libs/server/hub/session_routes_test.go`: `TestSessionRoutes_Lifecycle` (DB-backed) — real router: PAT-authed create(201)/list(200)/get(200)/close(204); runtime-authed result(204); result rejects a PAT with 401 (auth-split is load-bearing).

## Pre-existing failures (NOT caused by this change)

The following `libs/tests` failures reproduce identically on base `main`
(commit a4a72dd, which contains no c0031 code — verified by running them in the
`/d/mework` main worktree). This change touches no files under `libs/tests/`.

- `libs/tests/e2e` — `TestHEALTH_01_MissingSecretAbortsStartup` panics `design-only` (scaffold).
- `libs/tests/integration` — `TestMessageBus_PublishSseAckNoRedelivery` (invalid runtime token / claim route), `TestFullPipelineE2E_BehaviorPreservation` (409 runtime code already registered — data isolation), `TestChannelRouting_E2E` (auto-provision "runtime not found").

## Coverage (libs/server touched packages)

See `coverage.txt`. New code: `DispatchSessionToRunner` 72.2%, `CreateSession`
64.0%, `ResultSession` 60.0%, `NewHandlers` 100%.
