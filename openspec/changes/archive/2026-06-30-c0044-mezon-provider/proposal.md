## Why

Mework currently routes events only from **webhook-based issue trackers** (Mello kanban) through a request-reply job pipeline. There is no support for **real-time chat providers** like Mezon, where an agent can be reached conversationally — the way developers already interact with AI CLIs via `mework chat send`. Supporting Mezon as a provider/channel unlocks conversational agent access from any Mezon channel, making mework usable as a team AI assistant embedded in the team's existing chat platform, both in server-deployed and offline-local modes.

## What Changes

Introduce **Mezon** as a first-class provider and channel for mework, covering both the server-mode (hub + enrolled runners) and offline-mode (self-contained daemon) architectures:

- **Mezon provider adapter** — implement the `Provider` interface for Mezon: `Code()` returns `"mezon"`, `ChannelKey()` returns `("mezon", <channel_id>)`, plus event parsing, write-back (post messages to a Mezon channel), and message verification (bot token identity).
- **Mezon bot client** — a WebSocket-based client that authenticates via bot token (`appID:apiKey`), connects to Mezon's real-time gateway, receives messages from channels where the bot is present, and maintains heartbeat/ reconnect. Falls back to cursor-based polling when WebSocket is unavailable.
- **Channel routing for chat** — extend the channel router so Mezon channel messages are routed to per-channel agent sessions (like Mello ticket channels but conversational). The channel key is `"mezon:<channel_id>"`, and each channel gets its own session.
- **Server-mode integration** — the Mezon bot runs as a long-lived service inside `mework-server`, authenticates via configured credentials, subscribes to events, and feeds messages into the channel router. The existing `provider_connections` table stores the sealed bot credentials.
- **Offline-mode integration** — the offline daemon can start a Mezon bot listener (when configured) alongside the Unix socket, allowing Mezon users to chat with the agent without any hub/Postgres dependency. Messages arrive via WebSocket, the offline policy engine processes them, and replies are sent back to Mezon.
- **Config & secrets** — new env vars/config fields: `MEZON_BOT_APP_ID`, `MEZON_API_KEY`, `MEZON_BASE_URL` (optional, for self-hosted Mezon). Credentials sealed at rest via the existing AES-256-GCM mechanism.
- **CLI commands** — `mework config set mezon.app-id ...`, `mework config set mezon.api-key ...` for local config; `mework connection create mezon ...` for server-side credential storage.

### What does NOT change

- The trigger grammar (`@mework [profile] [workflow] [instructions]`) stays the same for webhook sources. Mezon channels use an **alternative trigger model**: every message in a bound channel is routed to the session (no `@mework` prefix needed). The bot can also support `@BotName <instruction>` for DM-triggered ad-hoc tasks.
- The legacy poll/queue pipeline is untouched. Mezon routing goes through the channel router (opt-in, disabled by default).
- The existing Mello adapter is unchanged.
- No breaking changes to the Provider interface — Mezon uses the same `Code()`, `ChannelKey()`, `WriteBack()`, `FetchTaskDetail()` methods. The new parts (WebSocket listener, message dispatch loop) live outside the adapter, in a `bot` or `listener` package.

## Capabilities

### New Capabilities
- `mezon-provider`: Mezon provider adapter implementing the Provider interface; webhookless event parsing, channel key extraction, and REST write-back (send messages to Mezon channels)
- `mezon-bot`: WebSocket-based bot client for receiving Mezon messages; authentication, event subscription, heartbeat, reconnection, and message dispatch to channel router

### Modified Capabilities
- `provider-gateway`: the adapter interface now supports **chat-type providers** alongside webhook-type providers; `ChannelKey` semantics are extended to cover real-time chat channels; connection resolution works for both write-back and bot-to-channel dispatch
- `channel-routing`: channel router extended to support **bidirectional real-time channels** (chat) alongside the existing event-based (ticket comment) routing; message format normalized across chat and webhook sources
- `offline-agent`: offline daemon can host a Mezon bot listener as an additional input channel (alongside the existing Unix socket); policy engine processes Mezon-sourced messages with sender/channel attributes
- `daemon-runtime`: daemon can enroll with a Mezon bot listener capability; runner can host its own bot connection for direct Mezon-to-sandbox dispatch (server-mode optimization)
- `session-channel-binding`: channel sessions can be created for Mezon channel IDs; lifecycle management works for chat channels (active while bot is present, closed on bot disconnect)

## Impact

- **New packages**: `libs/server/provider/mezon/` (adapter), `libs/server/provider/mezon/bot/` (WebSocket bot client, message loop)
- **New dependency**: `github.com/nccasia/mezon-go-sdk` (or equivalent) for Mezon REST API types; `github.com/gorilla/websocket` (already in go.sum as a transitive dep)
- **Config changes**: `libs/server/hub/config.go` gains optional `MezonAppID`, `MezonAPIKey`, `MezonBaseURL` fields
- **Schema changes**: none — existing `provider_connections`, `channel_sessions`, `account_identities` tables are provider-agnostic and accommodate `provider_code = "mezon"`
- **API changes**: no new public HTTP endpoints; the bot listener is internal (WebSocket outbound to Mezon, not inbound HTTP)
- **Offline mode**: `libs/client/runner/offline.go` extended to start a Mezon bot goroutine when configured; `libs/client/runner/offline_client.go` gets a Mezon message dispatch path
- **Configuration UX**: `mework config` subcommands for Mezon credentials; `mework connection create mezon` for server deployments
