# Mezon Worker

## ADDED Requirements

### Requirement: Worker enqueues Mezon messages as jobs

The worker SHALL connect to Mezon via WebSocket, receive channel messages (the bot library filters self-messages before dispatch), and enqueue each message as a job via the server API. The enqueue payload SHALL include the provider code `"mezon"`, the channel ID, sender ID, message text, and message ID for dedup.

#### Scenario: Message received and enqueued

- **WHEN** the worker receives a Mezon channel message from another user
- **THEN** it calls `POST /api/v1/jobs/enqueue` with `{provider_code: "mezon", channel_id, sender_id, text, message_id}`
- **THEN** the server creates a job with status `queued` and `provider_code = "mezon"`

#### Scenario: Enqueue endpoint returns server error

- **WHEN** the `POST /api/v1/jobs/enqueue` call returns a 5xx error
- **THEN** the worker logs the error and continues without crashing
- **THEN** the message is dropped (not retried or queued locally)

#### Scenario: Initial Mezon connection fails

- **WHEN** the worker cannot connect to Mezon WebSocket on startup due to invalid credentials or unreachable host
- **THEN** the worker logs the error and exits with a non-zero exit code

#### Scenario: Permanent disconnection after retries

- **WHEN** the Mezon WebSocket reconnection attempts exceed the retry limit
- **THEN** the inbound loop stops and logs a fatal error
- **THEN** the outbound loop continues operating independently

### Requirement: Worker polls for completed jobs at a configurable interval

The worker SHALL poll the server for completed jobs where `provider = "mezon"` at a configurable interval (default 5s).

#### Scenario: Poll and reply

- **WHEN** the worker polls `GET /api/v1/jobs?provider=mezon&status=done&since=<cursor>` and finds a completed job
- **THEN** it extracts the result and channel ID from the job
- **THEN** it posts the result as a message to the Mezon channel via the bot's `SendMessage()`
- **THEN** it advances the cursor past that job ID

### Requirement: Worker deduplicates replies via persisted cursor

The worker SHALL use a cursor to avoid processing the same job twice and SHALL persist the cursor to a local file for crash recovery.

#### Scenario: Cursor persisted across restarts

- **WHEN** the worker restarts after a crash
- **THEN** it reads the cursor from the local state file and resumes polling from that point
- **THEN** already-replied jobs before the cursor are not re-processed

#### Scenario: SendMessage failure does not advance cursor

- **WHEN** `SendMessage` returns an error during the outbound reply
- **THEN** the cursor is NOT advanced past that job ID
- **THEN** the job is re-processed on the next poll interval

### Requirement: Worker is configured via environment

The worker SHALL read its configuration from environment variables: `MEZON_APP_ID`, `MEZON_API_KEY`, `MEZON_BASE_URL` (optional), `MEWORK_SERVER_URL` (default `http://localhost:8080`), and `MEWORK_TOKEN` (runtime token for API auth).

#### Scenario: Start with minimal config

- **WHEN** the worker starts with `MEZON_APP_ID`, `MEZON_API_KEY`, `MEWORK_TOKEN`, and `MEWORK_SERVER_URL` set
- **THEN** it connects to Mezon and begins the inbound and outbound loops

#### Scenario: Missing credentials fails fast

- **WHEN** the worker starts without `MEZON_APP_ID`, `MEZON_API_KEY`, or `MEWORK_TOKEN`
- **THEN** it prints an error listing the missing variables and exits with a non-zero exit code

### Requirement: Isolated inbound and outbound loops

The worker SHALL run the inbound (receive -> enqueue) and outbound (poll -> reply) loops as independent concurrent tasks. A failure in one loop SHALL NOT affect the other.

#### Scenario: Inbound loop fails independently

- **WHEN** the Mezon WebSocket connection drops and the inbound loop reconnects
- **THEN** the outbound loop continues polling and replying without interruption

#### Scenario: Outbound poll error is logged

- **WHEN** the outbound loop's poll request to the server fails
- **THEN** the error is logged and the loop retries on the next interval
- **THEN** the inbound loop continues processing messages

### Requirement: Worker logs and continues on non-fatal errors

The worker SHALL log errors from either loop and continue operating. Only errors that make continued operation impossible (e.g., permanent credential rejection) SHALL cause the worker to exit.

#### Scenario: Server API transient error is logged

- **WHEN** a call to the server API returns a transient error (timeout, 503)
- **THEN** the worker logs the error and the affected loop retries
- **THEN** the worker does not exit

### Requirement: Worker sends replies via bot client

The worker SHALL send replies to Mezon channels by calling `bot.SendMessage()` directly, not through the mezon-provider adapter's WriteBack method. The outbound loop SHALL extract the channel ID and result text from the completed job and pass them to the bot client's `SendMessage(ctx, channelID, content)`.

#### Scenario: Outbound reply via bot

- **WHEN** the outbound loop finds a completed job for a Mezon channel
- **THEN** it extracts `channel_id` and the result text from the job record
- **THEN** it calls `bot.SendMessage(ctx, channelID, resultText)`
- **THEN** on success, it advances the cursor past that job

## REMOVED Requirements

### Requirement: Daemon can host a Mezon bot proxy

**Source**: `openspec/specs/daemon-runtime/spec.md`

**Reason**: The Mezon bot has been refactored into a standalone worker. The daemon no longer has any Mezon-related responsibility -- the worker communicates with the server via HTTP API, not via the daemon's SSE subscription.

**Migration**: The worker binary handles inbound message reception and outbound reply delivery. Daemon enrollment no longer supports `mezon_bot: true`.

### Requirement: Daemon spec includes mezon-bot capability

**Source**: `openspec/specs/daemon-runtime/spec.md`

**Reason**: Removed alongside the daemon's Mezon bot hosting capability. The worker is a separate binary and does not enroll with the daemon.

**Migration**: The `"mezon-bot"` spec string is no longer advertised by any daemon enrollment. The worker authenticates via `MEWORK_TOKEN`, not via runner enrollment specs.

### Requirement: Server embeds MezonBotService

**Source**: Server implementation (libs/server/hub/mezon_service.go, apps/mework-server/main.go)

**Reason**: `MezonBotService`, `SetupMezon()`, and all Mezon-related startup code are removed from the server process. The server no longer starts or manages the Mezon bot lifecycle.

**Migration**: The mework-mezon-worker binary handles all Mezon connectivity. Hub config fields `MezonAppID`, `MezonAPIKey`, `MezonBaseURL` are removed.

### Requirement: Server starts Mezon bot at startup

**Source**: Server implementation (hub/server.go SetupMezon, main.go Mezon section)

**Reason**: `SetupMezon()` and the Mezon startup code in `apps/mework-server/main.go` are deleted. The adapter is registered without a bot reference.

**Migration**: The server no longer embeds Mezon startup logic. Mezon connectivity is the worker's responsibility.

## MODIFIED Requirements

### Requirement: Mezon Provider adapter registration

The Mezon adapter SHALL register itself with the global provider registry without requiring a bot argument. The `RegisterAdapter()` function SHALL NOT accept a bot parameter.

**Source**: `openspec/specs/mezon-provider/spec.md` -- "Provider adapter registration"

**Change**: `RegisterAdapter()` no longer accepts a bot argument. The adapter is registered standalone and does not embed or receive a bot client reference. Server-side write-back to Mezon is no longer supported (the worker handles outbound replies via `bot.SendMessage()` directly).

#### Scenario: Register adapter without bot

- **WHEN** the server starts and registers the Mezon adapter
- **THEN** `RegisterAdapter()` is called without a bot argument
- **THEN** the adapter is registered in the global provider registry under code `"mezon"`

**Reason**: The bot is owned by the worker binary. The server-side adapter no longer needs a bot reference since it no longer performs write-back to Mezon.

**Migration**: Remove the bot parameter from `RegisterAdapter()` calls. Call sites that previously passed a bot now pass no bot.

### Requirement: Mezon Bot specification purpose

The mezon-bot spec Purpose line SHALL state that the bot client is used by the standalone mework-mezon-worker binary.

**Source**: `openspec/specs/mezon-bot/spec.md` -- Purpose line

**Change**: The bot client is used by the standalone mework-mezon-worker binary, not by the server-side channel router or the offline-mode agent. The Purpose line SHALL be updated accordingly.

#### Scenario: Purpose line reflects worker usage

- **WHEN** a developer reads the mezon-bot spec
- **THEN** the Purpose line states the bot client is used by the `mework-mezon-worker` binary
- **THEN** the Purpose line does not mention the server-side channel router or offline-mode agent

**Reason**: The Mezon bot has been moved out of the server process and into the standalone worker binary.
