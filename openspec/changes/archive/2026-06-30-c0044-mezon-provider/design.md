## Context

Mework today routes events exclusively from **webhook-based issue trackers** — currently only Mello, with GitHub/Jira stubs. The `Provider` interface, the webhook pipeline, and the channel router all assume an event-driven model: an external system POSTs a webhook, the server verifies the signature, parses the event, and either enqueues a job or routes to a channel session.

**Mezon is fundamentally different.** It is a real-time chat platform where messages arrive over a persistent WebSocket connection, not via webhooks. A Mezon bot:
1. Authenticates via `appID:apiKey` → receives a session token
2. Opens a WebSocket to `wss://<mezon-host>/ws?lang=en&status=true&token=<token>&format=protobuf`
3. Receives protobuf-encoded `ChannelMessage` events in real time
4. Sends messages by writing protobuf envelopes back over the WebSocket (or via REST)
5. Requires heartbeat pings every ~10s to keep the connection alive
6. Falls back to cursor-based REST polling when WebSocket is unavailable

This design adds **Mezon as a first-class provider and channel**, supporting both the server-mode hub architecture and the self-contained offline-mode daemon.

Existing infrastructure we reuse:
- `Provider` interface (for `Code()`, `ChannelKey()`, `WriteBack()`, `FetchTaskDetail()`)
- `channel.Router` + `channel.Registry` (for channel-addressed event routing)
- `provider_connections` table (for sealed bot credentials)
- `policy.Policy` + `policy.Attributes` (for offline-mode message filtering)
- The JSON-RPC Unix socket protocol (for offline-mode message dispatch)
- `connection.Service.GetDecryptedToken()` (for server-mode credential access)

## Goals / Non-Goals

**Goals:**
- Implement a `mezon` provider adapter that satisfies the `Provider` interface
- Build a WebSocket-based Mezon bot client that receives messages and dispatches them to the channel router (server mode) or the offline daemon (offline mode)
- Support per-channel agent sessions: each Mezon channel gets its own sandbox session, routed through the existing `channel.Router`
- Allow write-back: agent responses are posted back to the originating Mezon channel
- Support server-mode: bot runs inside `mework-server` as a long-lived goroutine service
- Support offline-mode: bot runs inside the offline daemon as an additional message source alongside the Unix socket
- Store Mezon bot credentials (appID, apiKey, optional baseURL) as a provider connection, sealed at rest
- CLI commands for credential management (`mework connection create mezon`, `mework config` for offline mode)

**Non-Goals:**
- Full Mezon platform API coverage (only message send/receive, channel listing, and basic user info — not audio, video, or admin APIs)
- Webhook path for Mezon (the bot's WebSocket connection is the primary event source; Mezon webhooks for third-party integration are out of scope)
- Implementing the Mezon turbo SDK's multi-tier hot/warm/cold architecture (one bot connection per deployment is sufficient for mework's use case)
- Breaking changes to the `Provider` interface or the channel router API
- Changing the Mello adapter or the webhook pipeline

## Decisions

### D1: Separate the bot client from the provider adapter

**Decision:** The `mezon` provider adapter (`libs/server/provider/mezon/adapter.go`) implements the `Provider` interface (parsing, channel key, write-back), while the WebSocket bot client lives in a separate `libs/server/provider/mezon/bot/` package.

**Rationale:**
- The `Provider` interface was designed for **event processing and write-back**, not for managing persistent connections
- The bot client has its own lifecycle (connect, authenticate, heartbeat, reconnect, dispatch) that is orthogonal to the adapter's stateless methods
- Separating them keeps the adapter simple (~150 lines, matching Mello's adapter) and the bot package focused on connection management
- In offline mode, the daemon only needs the bot client, not the full adapter

**Alternatives considered:**
- Embedding bot logic in the adapter — rejected because it conflates two concerns and makes the adapter stateful
- A single `libs/server/provider/mezon/` package with sub-packages — accepted, this is the chosen approach

### D2: Bot client uses the Mezon base SDK, not the turbo SDK

**Decision:** The bot client depends on `github.com/nccasia/mezon-go-sdk` (the base SDK) for authentication, WebSocket connection, and protobuf types, rather than `github.com/mezon/mezon-go-sdk-turbo`.

**Rationale:**
- The base SDK has ~3 direct dependencies (`gorilla/websocket`, `antihax/optional`, `golang.org/x/oauth2`) plus `google.golang.org/protobuf` — lightweight and auditable
- The turbo SDK adds Redis, rate limiting, tier management, and a multi-tenant state store — too heavy for mework's single-bot-per-deployment model
- The turbo SDK is a private module under active development with a local replace directive — importing it would create maintenance burden
- The base SDK provides exactly what we need: authentication, WebSocket dial, and protobuf message decoding
- We write a simpler dispatch loop (one connection, goroutine-per-message) rather than importing the turbo engine

### D3: Single WebSocket connection per deployment

**Decision:** Each `mework-server` instance maintains one WebSocket connection to Mezon (the bot's connection). The bot subscribes to all channels where it has presence.

**Rationale:**
- Mework is a single-tenant-per-deployment tool (one account, one bot). It does not need the hot/warm/cold multi-tenancy of the turbo SDK.
- One WebSocket connection is sufficient to receive messages from all channels the bot is in.
- If the connection drops, the bot retries with exponential backoff (1s, 2s, 4s, 8s, max 30s).
- The bot's user ID is derived from the authentication response and used for self-message filtering (the `self-retrigger guard` invariant).

### D4: Mezon uses the channel router, not the legacy job queue

**Decision:** Mezon messages are always routed through the `channel.Router`, never through the legacy `queued → claimed → running → done` job pipeline. The webhook endpoint (`POST /webhooks/{provider}`) is not used for Mezon.

**Rationale:**
- Mezon is a conversational chat platform, not a task-tracker. The job queue model (claim, execute, write-back) is designed for asynchronous one-shot tasks triggered by ticket comments.
- The channel routing model (bind session → route messages → session processes → write-back) is a natural fit for chat: each channel has a persistent session, and messages are streamed in real time.
- The channel router's `FeatureFlag` (opt-in, disabled by default) means Mezon routing is zero-risk for existing deployments.
- The `ChannelKey()` method returns `("mezon", "<channel_id>")`, which the channel router handles identically to `("mello", "TICKET-99")`.

### D5: Offline mode runs the bot in a goroutine alongside the Unix socket

**Decision:** In offline mode, the daemon's `Start()` method optionally starts a Mezon bot goroutine when Mezon credentials are configured in the workspace config or env vars.

**Rationale:**
- The offline daemon already has a `Start()` → `acceptLoop()` pattern for the Unix socket. The Mezon bot is another message source with the same lifecycle (start on `Start()`, stop on context cancellation).
- Messages from Mezon go through the same `policy.Policy` enforcement as Unix socket messages, with `"channel": "mezon:<channel_id>"` in the attributes instead of `"channel": "local"`.
- After policy approval, messages are dispatched to the same sandbox session via `session.sandbox.Exec()`.
- Replies are sent back via the bot client's `SendTo()` method.

### D6: Write-back uses the bot's WebSocket for replies

**Decision:** The Mezon adapter's `WriteBack()` method sends messages via the bot's WebSocket connection (or REST fallback), not through a separate HTTP client.

**Rationale:**
- The bot's WebSocket is already authenticated and connected — reusing it avoids establishing a new HTTP connection for every reply.
- The Mezon protocol supports sending messages over the same WebSocket used for receiving: `BuildSendEnvelope()` creates the protobuf envelope, and `conn.WriteMessage()` sends it.
- When the WebSocket is not available (reconnecting), the bot falls back to the REST API (`POST /channels/{id}/messages`).
- The bot package exposes a `SendMessage(ctx, channelID, content)` method that handles the WebSocket-vs-REST decision internally.

## Risks / Trade-offs

- **[R1] WebSocket reliability** → The bot client implements exponential backoff reconnection (1s–30s), heartbeat pings every 10s, and self-message filtering (sender ID != bot user ID). Unavailability during reconnection is bounded by the backoff cap.
- **[R2] No webhook fallback** → If the Mezon WebSocket API changes or is unavailable, the bot degrades to REST polling (cursor-based, configurable interval). This is a degraded mode (higher latency, more API calls), not a failure.
- **[R3] Single connection is a bottleneck** → For a deployment with 100+ concurrent channels all receiving messages rapidly, one WebSocket connection may become CPU-bound on protobuf decoding. Mitigation: the connection handle loop decodes only field 6 (channel_message) from the protobuf envelope with a partial decoder, and message handlers are dispatched to a goroutine pool (default 8 workers, configurable).
- **[R4] Bot credentials at rest** → Mezon bot credentials (appID, apiKey) are stored in the same `provider_connections` table as Mello tokens, encrypted with the existing AES-256-GCM `Seal()` mechanism. The server's `MEWORK_SECRET_KEY` env var is required for both. No additional secret management is introduced.
- **[R5] Protobuf dependency** → The Mezon API uses protobuf for WebSocket messages. `google.golang.org/protobuf` is already a transitive dependency (via `mezon-go-sdk`). No new language or build tooling is introduced.
- **[R6] Offline mode credential storage** → In offline mode, there is no database to store credentials. The user provides Mezon credentials via `mework.yml` or env vars (`MEZON_APP_ID`, `MEZON_API_KEY`). These are held in memory only and never persisted to disk unencrypted. The workspace config file is the user's responsibility to secure.
