## Why

The Mezon integration shipped in c0044 embeds the bot client inside the server process (`MezonBotService`, `SetupMezon`, main.go startup). This couples the bot's lifecycle to the server's, bypasses the job queue via the channel router, and mixes In (receive) and Out (reply) flows into one path. The result is a fragile, tightly-coupled design that cannot start/stop independently, cannot scale separately, and breaks the provider-agnostic invariant that all providers communicate through the job queue. Mezon should be a **standalone worker** that enqueues jobs like any other provider and handles its own write-back independently.

## What Changes

- **New standalone worker binary** (`apps/mework-mezon-worker/`) — a separate process that connects to Mezon via WebSocket, receives messages, enqueues them as jobs via the server API, and independently polls for completed jobs to post replies back to Mezon channels.
- **Remove server-embedded integration** — delete `MezonBotService`, `SetupMezon()`, and the Mezon startup code from `apps/mework-server/main.go`. The server no longer starts or manages the Mezon bot.
- **In/Out separation** — two independent loops inside the worker:
  * **Inbound loop** (receive): Mezon WS → enqueue job via `POST /api/v1/jobs` → server queues it → daemon claims & processes
  * **Outbound loop** (reply): Poll server for completed jobs (`provider=mezon`, `status=done`) → format result → post to Mezon channel via bot's `SendMessage()`
- **Offline mode rework** — the offline daemon keeps its Unix socket for local interaction. Mezon in offline mode becomes a separate concern (the worker needs the hub, so offline mode doesn't support Mezon — the user runs the worker as a separate process pointed at a local server).
- **Adapter stays** — `mezon-provider` adapter remains registered on the server for event parsing and channel key extraction. The adapter's `RegisterAdapter()` no longer accepts a bot argument. Outbound replies are sent by the worker via `bot.SendMessage()` directly, not through the adapter's WriteBack.
- **Bot client stays in libs** — `libs/server/provider/mezon/bot/` remains as the shared bot client library, used by the worker binary.

### Breaking changes
- **BREAKING**: `MezonBotService`, `SetupMezon()`, and `hub.Config.Mezon*` fields are **removed**. Any external code depending on these will break.
- **BREAKING**: Offline mode no longer embeds a Mezon bot. The `MezonBot` interface in `offline.go` and `runMezonBot()` are **removed**.
- **BREAKING**: `mezonadapter.RegisterAdapter()` no longer accepts a bot argument. Callers must update their invocation to remove the bot parameter.

## Capabilities

### New Capabilities
- `mezon-worker`: standalone worker binary that connects to Mezon via WebSocket, enqueues jobs via the server API, and replies to Mezon channels with results

### Modified Capabilities
- `mezon-bot`: the bot client wrapper is no longer started by the server; it is imported and used by the worker binary. The Purpose line is modified to reflect this (was "used by server-side channel router and offline-mode agent").
- `mezon-provider`: adapter is kept; the adapter's `RegisterAdapter()` no longer accepts a bot argument. The worker sends outbound replies via `bot.SendMessage()` directly, not through the adapter's WriteBack.
- `offline-agent`: offline mode no longer supports Mezon bot embedding. The `MezonBot` interface and `runMezonBot()` are removed.
- `daemon-runtime`: daemon no longer hosts a Mezon bot proxy or advertises the mezon-bot spec capability. Two Mezon-related requirements (Mezon bot proxy hosting, mezon-bot spec capability) are removed from the daemon-runtime spec.

## Impact

- **New binary**: `apps/mework-mezon-worker/main.go` — standalone Go binary, built by `make build`
- **Removed packages**: `libs/server/hub/mezon_service.go` (deleted), `hub/server.go` `SetupMezon` removed, `apps/mework-server/main.go` Mezon startup removed
- **Removed fields**: `hub.Config.MezonAppID`, `MezonAPIKey`, `MezonBaseURL` removed
- **Removed interface method**: `MezonBot.OnMessage()` removed from `libs/client/runner/offline.go`; the `MezonBot` interface itself is removed
- **Modified**: `libs/client/runner/offline.go` — remove `runMezonBot()` and `MezonBot` interface
- **Server API**: the worker needs a job enqueue endpoint (`POST /api/v1/jobs`) and a job poll endpoint (`GET /api/v1/jobs?provider=mezon&status=done`). These may already exist or need to be added.
- **Worker config**: the worker reads `MEZON_APP_ID`, `MEZON_API_KEY`, `MEZON_BASE_URL`, and `MEWORK_SERVER_URL` from env
- **Dependencies**: the worker imports `libs/server/provider/mezon/bot` (already vendored)
