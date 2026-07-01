# REST Write-Back Specification

## Purpose

Define how a finished job's result is written back to the originating provider.
Write-back is performed **server-side over the provider's REST API** (not by the
daemon, and not via MCP), through a durable outbox so results are delivered
exactly once even across restarts. Owned by `internal/server/jobs/writeback.go`
and the provider adapter's write-back method.

## Requirements

### Requirement: Server-side REST write-back

The system SHALL perform result write-back on the server using the provider
adapter's REST client (e.g. Mello `CreateComment`), unsealing the provider
connection credential only at write time. The daemon MUST NOT hold provider
credentials or perform write-back itself.

#### Scenario: Post the result back to the provider

- **WHEN** a job is acked `done` with a result summary
- **THEN** the server calls the provider's REST API to post the result as a comment on the source ticket

#### Scenario: Daemon holds no write-back credentials

- **WHEN** the daemon completes a job
- **THEN** it reports the result to the server only, and the server owns the credentialed write-back

### Requirement: Durable outbox delivery

The system SHALL track write-back state durably (e.g. `pending → processing →
done`/`failed`) so that a crash or restart does not drop or duplicate a
write-back, and the sweeper retries pending write-backs.

#### Scenario: Retry after a transient failure

- **WHEN** a write-back attempt fails transiently
- **THEN** the outbox keeps it pending and the sweeper retries it later

#### Scenario: No duplicate comment on restart

- **WHEN** the server restarts after a write-back has already been delivered
- **THEN** the result is not posted to the provider a second time

### Requirement: Write-back via channel session context

When a sandbox completes processing on a channel, the write-back SHALL use the channel session context (provider code, account ID, resource ID) to look up the provider connection, decrypt the token server-side, and post the result. The runner SHALL NOT hold the provider token.

#### Scenario: Write-back from channel session

- **WHEN** a sandbox finishes processing channel `"mello:TICKET-99"` and produces a result
- **THEN** the server looks up the channel session, resolves the provider connection using the session's account ID and provider code, decrypts the token, and calls the Mello adapter's `WriteBack`
