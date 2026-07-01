# REST Write-back Specification — Delta

## ADDED Requirements

### Requirement: Write-back via channel session context

When a sandbox completes processing on a channel, the write-back SHALL use the channel session context (provider code, account ID, resource ID) to look up the provider connection, decrypt the token server-side, and post the result. The runner SHALL NOT hold the provider token — the existing security invariant is preserved.

#### Scenario: Write-back from channel session

- **WHEN** a sandbox finishes processing channel `"mello:TICKET-99"` and produces a result
- **THEN** the server looks up the channel session, resolves the provider connection using the session's account ID and provider code, decrypts the token, and calls the Mello adapter's `WriteBack` with the result

#### Scenario: No token leakage to sandbox

- **WHEN** a sandbox is provisioned for any channel
- **THEN** the provider token is never passed to the sandbox process or included in channel events
