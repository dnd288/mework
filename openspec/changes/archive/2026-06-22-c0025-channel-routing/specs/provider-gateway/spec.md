# Provider Gateway Specification — Delta

## ADDED Requirements

### Requirement: Adapter exposes channel key method

The provider adapter interface SHALL be extended with a method `ChannelKey(event payload) -> (providerCode, resourceID)` that extracts the normalized channel tuple from a raw event. This enables the channel router to compute the channel key without provider-specific knowledge. All existing adapters SHALL implement this method.

#### Scenario: Mello adapter returns channel key

- **WHEN** the Mello adapter's `ChannelKey` is called with a webhook payload containing `ticket_id = "TICKET-99"`
- **THEN** it returns `("mello", "TICKET-99")`

#### Scenario: Channel key is used for routing

- **WHEN** a GitHub adapter (future) implements `ChannelKey` returning `("github", "42")`
- **THEN** the channel router routes the event using that key without knowing it came from GitHub

### Requirement: Provider connection resolved from channel session

The provider connection lookup SHALL be extended to support resolution from a channel session context (account ID + provider code), in addition to the existing direct lookup. This enables the write-back flow to find the correct credentials from the channel binding without the caller needing to know the account ID explicitly.

#### Scenario: Write-back resolves connection from session

- **WHEN** a write-back is triggered for channel `"mello:TICKET-99"` with a bound session containing `account_id = "acct_1"`
- **THEN** the system looks up the provider connection using `(account_id="acct_1", provider_code="mello")` and unseals the credential
