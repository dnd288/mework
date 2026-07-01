## 1. Server API Endpoints

- [x] 1.1 Add `POST /api/v1/jobs/enqueue` endpoint — accepts `{provider_code, channel_id, sender_id, text, message_id}`, validates, calls `orchestrator.Enqueue()`, returns job ID. Handles dedup via `(provider_code, external_event_id)` unique constraint
- [x] 1.2 Add `GET /api/v1/jobs?provider=<code>&status=<status>&since=<cursor>` endpoint — returns paginated jobs matching the filter, ordered by creation time. Used by the worker's outbound loop to find completed jobs

## 2. Standalone Worker Binary (`apps/mework-mezon-worker/`)

- [x] 2.1 Create `apps/mework-mezon-worker/main.go` — loads config from env (`MEZON_APP_ID`, `MEZON_API_KEY`, `MEZON_BASE_URL`, `MEWORK_SERVER_URL`, `MEWORK_TOKEN`), validates required fields, starts inbound and outbound loops as goroutines, handles graceful shutdown
- [x] 2.2 Implement inbound loop — connects to Mezon via `mezonbot.Bot`, on each received message calls `POST /api/v1/jobs/enqueue` with the message data, logs errors without crashing
- [x] 2.3 Implement outbound loop — polls `GET /api/v1/jobs?provider=mezon&status=done&since=<cursor>` at configurable interval (default 5s), for each completed job posts reply to Mezon channel via `bot.SendMessage()`, advances cursor; persists cursor to local file for crash recovery
- [x] 2.4 Add `apps/mework-mezon-worker/main.go` to `make build`

## 3. Remove Server-Embedded Mezon Code

- [x] 3.1 Delete `libs/server/hub/mezon_service.go` — `MezonBotService` and `NewMezonBotService` are removed
- [x] 3.2 Remove `SetupMezon()` from `libs/server/hub/server.go` — the adapter `RegisterAdapter()` is called by the worker or at server startup without a bot
- [x] 3.3 Remove `MezonBotService` startup from `apps/mework-server/main.go` — the Mezon section (4a) and related shutdown code are removed
- [x] 3.4 Remove `MezonAppID`, `MezonAPIKey`, `MezonBaseURL` from `libs/server/hub/config.go` and `LoadConfig()` — the server no longer stores Mezon credentials
- [x] 3.5 Update `mezonadapter.RegisterAdapter()` — it no longer receives a bot argument; the adapter is registered standalone (write-back is handled by the worker)

## 4. Remove Offline Mode Mezon Support

- [x] 4.1 Remove `MezonBot` interface from `libs/client/runner/offline.go`
- [x] 4.2 Remove `SetMezonBot()` method from `OfflineServer`
- [x] 4.3 Remove `mezonStarted` field and `runMezonBot()` method from `OfflineServer`
- [x] 4.4 Remove Mezon bot goroutine startup from `OfflineServer.Start()`
- [x] 4.5 Remove `MezonConfigFromWorkspace()` from `libs/client/runner/offline_client.go`
- [x] 4.6 Remove `mezon` config fields from `meworkYMLConfig` in `offline_client.go`
- [x] 4.7 Remove `mezon.app_id` / `mezon.api_key` from the CLI config commands (`cmd_config.go`)

## 5. Clean Up Tests

- [x] 5.1 Remove offline Mezon integration tests from `libs/tests/integration/mezon_offline_test.go` — the offline mode no longer supports Mezon
- [x] 5.2 Update `libs/tests/integration/mezon_channel_routing_test.go` — remove tests that depend on `MezonBotService` or server-embedded bot; keep adapter tests
- [x] 5.3 Add tests for the new `POST /api/v1/jobs/enqueue` endpoint (DB-backed integration test)
- [x] 5.4 Add tests for the new `GET /api/v1/jobs?provider=&status=&since=` endpoint (DB-backed integration test)
- [x] 5.5 Add unit tests for the worker's inbound/outbound loops with mock server and mock Mezon bot
- [x] 5.6 Restore or fix any adapter tests that broke due to `RegisterAdapter()` signature change

## 6. Docs and Build

- [x] 6.1 Update `docs/` to document the new worker binary and its In/Out separation architecture
- [x] 6.2 Update `Makefile` to build `mework-mezon-worker`
- [x] 6.3 Verify `go build ./...`, `go vet ./...`, and `openspec validate` pass cleanly
