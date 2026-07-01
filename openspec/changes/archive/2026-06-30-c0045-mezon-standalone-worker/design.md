## Context

c0044 shipped the Mezon integration with the bot client embedded in the server process (`MezonBotService`, `SetupMezon`, main.go startup). Messages were routed through the channel router (not the job queue), and the bot's lifecycle was tied to the server's. This creates several problems:

- **Coupling**: the server cannot start without Mezon credentials being present (or work around a nil SDK client)
- **No In/Out separation**: the same process that receives messages also sends replies, making independent scaling impossible
- **Bypasses the job queue**: the channel router delivers directly to sessions, skipping the durable queue that provides retries, dedup, and async processing for every other provider
- **Provider inconsistency**: Mello uses the proven webhook → enqueue → claim → write-back pipeline. Mezon should follow the same pattern, just with a different event source (WebSocket instead of webhook)

This change refactors Mezon into a **standalone worker** that communicates with mework-server only through the job queue API, matching Mello's architectural pattern.

## Goals / Non-Goals

**Goals:**
- Extract a standalone worker binary (`apps/mework-mezon-worker/`) with two independent loops: inbound (enqueue) and outbound (reply)
- Remove all server-embedded Mezon code (`MezonBotService`, `SetupMezon`, `hub.Config.Mezon*`, `main.go` startup)
- Remove the `MezonBot` interface and `runMezonBot()` from the offline daemon
- Keep the shared bot client library (`libs/server/provider/mezon/bot/`) and the adapter (`libs/server/provider/mezon/adapter.go`)
- Add server API endpoints for the worker: job enqueue and job poll
- Match the Mello pattern: event source → job queue → daemon → write-back

**Non-Goals:**
- Changing the Mello adapter or webhook pipeline
- Full Mezon platform API coverage (only messaging)
- Multi-worker scaling or load balancing (one worker is sufficient)
- The offline mode will not support Mezon (the worker requires a running hub)

## Decisions

### D1: Worker communicates via HTTP API, not message bus directly

**Decision:** The worker enqueues jobs and polls for results via HTTP API endpoints on mework-server, not by subscribing directly to bus topics.

**Rationale:**
- The existing daemon already uses `POST /api/v1/jobs/claim`, `POST /api/v1/jobs/{id}/ack` for the job lifecycle. Adding an enqueue endpoint is consistent.
- HTTP is simpler to auth, test, and debug than bus subscriptions.
- The server remains the single authority on job state (DB-backed).
- The worker can be written as a simple request-response loop without bus client dependencies.

**Alternatives considered:**
- Bus subscription directly to channel topics — rejected because it would couple the worker to the message bus implementation and bypass job durability.
- Unix socket IPC — rejected because the worker should be able to run on a different host.

### D2: Worker uses a simple enqueue endpoint, not the webhook handler

**Decision:** The worker calls a new lightweight `POST /api/v1/jobs/enqueue` endpoint on the server, not `POST /webhooks/{provider}`.

**Rationale:**
- The webhook endpoint expects provider-specific signature headers and webhook payloads. Mezon messages arrive via WebSocket, not HTTP webhooks, so signature verification is inapplicable.
- A dedicated enqueue endpoint lets the worker pass structured JSON directly: `{provider_code, channel_id, sender_id, text, message_id}`.
- The endpoint reuses the same `orchestrator.Enqueue()` logic as the webhook handler, ensuring identical job semantics (queued → claimed → running → done).

### D3: Outbound loop polls, does not stream

**Decision:** The outbound loop polls `GET /api/v1/jobs?provider=mezon&status=done&since=<cursor>` at a configurable interval (default 5s). It does not subscribe to an SSE stream.

**Rationale:**
- Polling is simpler and more robust for a "check and reply" pattern. There's no requirement for sub-second reply latency.
- The worker is stateless — it can crash and restart without losing messages (the cursor tracks the last polled job).
- SSE would require the worker to maintain a persistent connection and handle reconnection logic, adding complexity.

**Alternatives considered:**
- SSE stream of completed jobs — simpler for the worker (push-based) but requires persistent connection management. Would be a good future optimization.
- Bus subscription — couples the worker to the bus implementation.

### D4: Offline mode drops Mezon support

**Decision:** The offline daemon (`mework start --offline`) does not support Mezon. The `MezonBot` interface, `SetMezonBot()`, and `runMezonBot()` are removed from `offline.go`. Mezon requires a running hub server.

**Rationale:**
- The offline mode's purpose is zero-infrastructure local operation. Adding Mezon requires network access to Mezon's servers and a hub for job queue durability.
- Offline mode keeps the Unix socket for local CLI interaction — that's its natural use case.
- Running the worker alongside a locally-started server (`mework server start`) already covers the "local Mezon" use case.

### D5: Worker credentials via environment variables, not sealed store

**Decision:** The worker reads `MEZON_API_KEY` and `MEWORK_TOKEN` from environment variables in plaintext, not from the server's sealed credential store (AES-256-GCM).

**Rationale:**
- The bot client needs the raw API key at startup to authenticate with Mezon's API. The sealed credential store lives in the server process and is unsealed only at write-back time.
- The worker is a sibling process running in the same trusted environment (same host or same orchestrated deployment) as the server. The env vars are passed by the deployment mechanism (systemd, Docker Compose, Kubernetes), which is responsible for secure injection.
- The `MEWORK_TOKEN` is a runtime token scoped to API access on the worker's own server, not a long-term credential.

**Alternatives considered:**
- Unseal API via server endpoint -- rejected: adds a startup dependency (server must be up before worker can start) and exposes the key over the network.
- File-based credential -- rejected: does not materially improve security over env vars; env vars are the Go standard for config injection (CLAUDE.md already uses env for `SERVER_KEY`, `MEWORK_SECRET_KEY`).
- Using the adapter's WriteBack -- rejected (see D6): the worker talks directly to the bot client, not through the adapter interface.

**Security posture:** The worker is a trusted process. The same deployment boundary that protects `MEWORK_SECRET_KEY` (the server's encryption key) protects the worker's environment variables. This is a permanent design choice, not a temporary workaround.

### D6: Adapter registration drops bot argument

**Decision:** `mezonadapter.RegisterAdapter()` no longer accepts a bot argument. The adapter is registered standalone; the worker owns the bot client.

**Rationale:**
- The server no longer hosts the bot client, so passing one at registration would require the server to import the bot package solely to pass a nil argument.
- The adapter's role is reduced to channel key extraction, event parsing, and no-op interface methods (WebhookHeaders, ExtractContainerID). Write-back is handled by the worker via `bot.SendMessage()` directly.
- Keeping the parameter would create a misleading API (callers must pass nil) without any benefit.

**Alternatives considered:**
- Keep the parameter but ignore it (pass nil from the server) -- rejected: creates a misleading API signature that suggests the bot is available when it is not.
- Split registration from bot attachment (two-phase init) -- rejected: adds complexity for no current use case; the worker does not use the adapter at all.
- Backward-compatible wrapper (RegisterAdapterWithBot and RegisterAdapterStandalone) -- rejected: YAGNI -- no existing callers depend on the bot argument since the server was the only call site.

## Risks / Trade-offs

- **[R1] Job latency** — The inbound loop enqueues a job, then the daemon must claim and process it before the outbound loop can reply. Total latency = enqueue + queue wait + daemon process + poll interval. Mitigation: the poll interval is configurable (default 5s), and the enqueue endpoint is immediate.
- **[R2] Polling overhead** — The outbound loop polls the server every N seconds. For low-traffic deployments this is wasteful. Mitigation: the poll interval defaults to 5s which is negligible load. Future optimization: SSE push.
- **[R3] Duplicate replies** — If the worker crashes after posting a reply but before recording the cursor, it may reply again to the same job on restart. Mitigation: the outbound loop records the cursor (last-processed job ID) to a local file after each successful reply, and uses `since=<cursor>` on the next poll.
- **[R4] No offline Mezon** — Users who want Mezon locally must also start the hub server. This is a trade-off accepted in D4.
