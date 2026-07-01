# Mezon Bot

## ADDED Requirements

### Requirement: Bot authentication

The bot client SHALL authenticate with Mezon using `appID` and `apiKey` credentials. It SHALL exchange these for a session token via `POST /v2/apps/authenticate/token` with HTTP Basic auth (`appID:apiKey`). The session token SHALL be retained and used for the WebSocket handshake and REST API calls.

#### Scenario: Successful authentication

- **WHEN** the bot client connects with valid `appID` and `apiKey`
- **THEN** it receives a session token with a non-empty `Token`, `RefreshToken`, and `UserID`

#### Scenario: Authentication failure

- **WHEN** the bot client connects with invalid credentials
- **THEN** it returns an authentication error and does not attempt a WebSocket connection

### Requirement: WebSocket connection

The bot client SHALL open a WebSocket connection to Mezon's real-time gateway using the session token. The WebSocket URL SHALL use the format `wss://<mezon-host>/ws?lang=en&status=true&token=<token>&format=protobuf`. The connection SHALL use the `"protobuf"` subprotocol.

#### Scenario: WebSocket handshake succeeds

- **WHEN** the bot client opens a WebSocket with a valid session token
- **THEN** the connection is established and the bot receives protobuf-encoded events

#### Scenario: WebSocket handshake fails

- **WHEN** the bot client opens a WebSocket with an expired or invalid session token
- **THEN** the bot client re-authenticates, obtains a fresh token, and retries the connection

### Requirement: Heartbeat and keepalive

The bot client SHALL send ping frames every 10 seconds to keep the WebSocket connection alive. Each ping SHALL carry an incrementing connection ID (`cid`). If no pong is received within 10 seconds of a ping, the connection SHALL be considered dead and the bot SHALL initiate reconnection.

#### Scenario: Periodic ping

- **WHEN** the bot has been connected for 10 seconds
- **THEN** it sends a ping frame with an incrementing `cid`

#### Scenario: Pong timeout triggers reconnect

- **WHEN** a ping is not acknowledged within 10 seconds
- **THEN** the bot closes the connection and initiates reconnection

### Requirement: Clan/channel event subscription

After WebSocket connection, the bot SHALL send a `ClanJoin` envelope for each clan the bot is a member of, including the special clan ID `"0"` for direct messages. This subscribes the bot to all channel events in those clans.

#### Scenario: Join clans on connect

- **WHEN** the bot connects and authenticates
- **THEN** it sends a `ClanJoin` envelope for each accessible clan

#### Scenario: Receive channel messages

- **WHEN** a message is sent to a channel the bot has joined
- **THEN** the bot receives a protobuf `ChannelMessage` event on the WebSocket

### Requirement: Self-message filtering

The bot SHALL ignore messages whose sender ID matches the bot's own user ID (obtained during authentication). This prevents feedback loops where a bot-written reply re-triggers processing.

#### Scenario: Skip self-authored messages

- **WHEN** the bot receives a message where `sender_id == bot_user_id`
- **THEN** the message is silently discarded and not dispatched

### Requirement: Automatic reconnection

The bot client SHALL automatically reconnect when the WebSocket connection drops. Reconnection SHALL use exponential backoff: 1s, 2s, 4s, 8s, up to a maximum of 30s between attempts. The bot SHALL re-authenticate and obtain a fresh session token on each reconnection attempt.

#### Scenario: Reconnect after connection drop

- **WHEN** the WebSocket connection is closed unexpectedly
- **THEN** the bot waits (with exponential backoff), re-authenticates, and reconnects

#### Scenario: Backoff cap

- **WHEN** the bot has reconnection-failed 6+ times consecutively
- **THEN** the retry interval is capped at 30 seconds (not growing further)

### Requirement: Message dispatch callback

The bot client SHALL accept a message dispatch callback function that is invoked for every received Mezon message (after self-message filtering). The callback receives the parsed message with channel ID, sender ID, message text, and any mentions.

#### Scenario: Dispatch received message

- **WHEN** the bot receives a valid channel message from another user
- **THEN** it calls the dispatch callback with the message details

### Requirement: Send message to channel

The bot client SHALL expose a `SendMessage(ctx, channelID, content)` method that sends a text message to a Mezon channel. It SHALL prefer sending over the active WebSocket connection. When the WebSocket is unavailable, it SHALL fall back to the Mezon REST API.

#### Scenario: Send over WebSocket

- **WHEN** `SendMessage` is called and the WebSocket is connected
- **THEN** the message is sent as a protobuf envelope over the WebSocket

#### Scenario: Send via REST fallback

- **WHEN** `SendMessage` is called and the WebSocket is not connected
- **THEN** the message is sent via the REST API (`POST /channels/{id}/messages`)

### Requirement: REST polling fallback

When the WebSocket connection cannot be established or maintained, the bot client SHALL fall back to cursor-based REST polling to receive messages. The polling interval SHALL be configurable (default 10 seconds). The bot SHALL track the last-seen cursor per channel to avoid duplicate delivery.

#### Scenario: Poll for messages

- **WHEN** the bot is in REST polling mode
- **THEN** it queries the Mezon API for new messages since the last cursor, per channel

#### Scenario: No duplicate delivery

- **WHEN** the bot polls the same message twice
- **THEN** the second delivery is suppressed (dedup by message ID)

### Requirement: Graceful shutdown

The bot client SHALL support graceful shutdown via context cancellation. On shutdown, it SHALL close the WebSocket connection cleanly, flush any pending outbound messages, and block until shutdown is complete (or a 5-second timeout elapses).

#### Scenario: Shutdown on context cancel

- **WHEN** the parent context is cancelled
- **THEN** the bot client closes the WebSocket, stops reconnection, and returns from `Start()`
