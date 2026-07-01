## 1. Shared Foundation

- [x] 1.1 Add `mezon-go-sdk` dependency to `go.work` / `go.mod` — specifically `github.com/nccasia/mezon-go-sdk` for REST client, WebSocket types, and protobuf message types
- [x] 1.2 Define shared Mezon types in `libs/shared/providers/mezon/` — `Config` struct (AppID, APIKey, BaseURL), `Message` struct (ChannelID, SenderID, Text, Mentions), and helpers for URL resolution (REST base, WebSocket gateway)
- [x] 1.3 Add `MezonAppID`, `MezonAPIKey`, `MezonBaseURL` optional fields to `libs/server/hub/config.go` and `LoadConfig()` with env var bindings (`MEZON_APP_ID`, `MEZON_API_KEY`, `MEZON_BASE_URL`)

## 2. Mezon Bot Client (`libs/server/provider/mezon/bot/`)

- [x] 2.1 Implement `Bot` struct with `NewBot(appID, apiKey, baseURL)` constructor and `Authenticate()` method — exchanges credentials for a session token via `POST /v2/apps/authenticate/token` with HTTP Basic auth; caches token with refresh logic
- [x] 2.2 Implement WebSocket connection lifecycle — `Connect()` dials `wss://<host>/ws?lang=en&status=true&token=<token>&format=protobuf`, sends ping frames every 10s with incrementing `cid`, handles pong timeout; `Disconnect()` closes cleanly
- [x] 2.3 Implement automatic reconnection — exponential backoff (1s, 2s, 4s, 8s, max 30s), re-authentication on reconnect, `IsConnected()` health check
- [x] 2.4 Implement message receiving — partial protobuf decoder that reads field 6 (channel_message) from envelopes; self-message filtering (sender ID != bot user ID); dispatch callback invoked per message with `(channelID, senderID, text, mentions)`
- [x] 2.5 Implement `SendMessage(ctx, channelID, text)` — sends text as protobuf envelope over WebSocket when connected; falls back to REST `POST /channels/{id}/messages` when WebSocket unavailable
- [x] 2.6 Implement REST polling fallback — cursor-based polling via `ListSince()` when WebSocket is unavailable; configurable poll interval (default 10s); dedup by message ID
- [x] 2.7 Implement graceful shutdown — context cancellation triggers clean WebSocket close, stops reconnection loop, blocks up to 5s for pending sends

## 3. Mezon Provider Adapter (`libs/server/provider/mezon/`)

- [x] 3.1 Implement `MezonAdapter` struct satisfying the `Provider` interface — `Code()` returns `"mezon"`, `ChannelKey()` extracts channel ID from message payload, `ParseEvent()` converts to `CanonicalEvent`, `WriteBack()` delegates to bot's `SendMessage()`, `FetchTaskDetail()` returns empty, `VerifyWebhook()`/`WebhookHeaders()`/`ExtractContainerID()` return no-op values
- [x] 3.2 Register `MezonAdapter` with the global `provider.Register()` during server startup (in hub server initialization, conditional on Mezon config being present)
- [x] 3.3 Implement Mezon credential management in connection service — `POST /api/v1/connections` stores Mezon appID in `config` JSONB and seals apiKey in `mcp_auth_enc`; `GetDecryptedToken()` unseals apiKey for bot startup

## 4. Server-Mode Integration

- [x] 4.1 Add `MezonBotService` to hub server — a goroutine-based service that starts/stops with the server; creates bot client from the first Mezon provider connection; logs connection status and reconnection events
- [x] 4.2 Wire bot dispatch callback to channel router — when the bot receives a message, call `router.Route(ctx, "mezon", channelID, "message.created", payload)`, where payload is the serialized `CanonicalEvent`
- [x] 4.3 Implement server-side write-back path — when a channel session completes and write-back is triggered, the Mezon adapter's `WriteBack()` sends the result to the channel via the bot's `SendMessage()` (the bot instance is accessible from the service)

## 5. Offline-Mode Integration

- [x] 5.1 Extend `OfflineServer` to start a Mezon bot goroutine when credentials are present — reads `mework.yml` for `mezon.app_id`, `mezon.api_key`, `mezon.base_url`; creates bot client and starts it in a goroutine; logs "Mezon bot connected" or "Mezon bot unavailable" at startup
- [x] 5.2 Wire offline Mezon messages to policy enforcement — `OfflineServer.handleConnection()` pattern reused: when the bot receives a message, evaluate `policy.Policy` with attributes `channel: "mezon:<id>"`, `sender`, `content`, `content_length`; blocked messages get an error reply sent to the Mezon channel
- [x] 5.3 Wire approved offline Mezon messages to sandbox — after policy pass, call `session.sandbox.Exec()` with the message text; capture output and send reply via bot's `SendMessage()` to the originating channel

## 6. CLI Commands

- [x] 6.1 Support `mework connection create mezon` — prompts for `app_id` and `api_key` (or reads from `--app-id` / `--api-key` flags); creates a provider connection with `provider_code = "mezon"`; validates by attempting authentication with the Mezon API
- [x] 6.2 Support `mework config set mezon.app-id` / `mezon.api-key` for offline-mode credential storage — writes to `~/.mework/config.yml` with file perms `0600`; used by the offline agent's bot startup path

## 7. Testing

- [x] 7.1 Unit tests for bot client — mock WebSocket server that responds to authentication and echoes messages; verify connect, send, receive, reconnect, self-message filter, and shutdown
- [x] 7.2 Unit tests for Mezon provider adapter — verify `Code()`, `ChannelKey()`, `ParseEvent()`, `WriteBack()`, `FetchTaskDetail()`, `VerifyWebhook()` no-op behaviors
- [x] 7.3 Integration test: channel routing from Mezon message source — simulated bot dispatch → channel router → session → write-back, verifying end-to-end message flow using in-memory bus and mock provisioner
- [x] 7.4 Integration test: offline mode with Mezon input — start offline agent with mock Mezon bot, send message, verify policy enforcement and sandbox dispatch
