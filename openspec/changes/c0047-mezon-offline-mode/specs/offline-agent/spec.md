# MODIFIED **offline-agent**

## MODIFIED Requirements

### Requirement: Offline mode requires zero external infrastructure  *(modified)*

The offline-mode agent SHALL NOT depend on Postgres, a hub server, a provider
adapter, or any network-accessible service. All state SHALL be in-memory or on
the local filesystem. The agent SHALL NOT attempt to connect to `MEWORK_HUB_URL`
or register as a runner. No PAT, rt_token, or enrollment is required.

The above describes the **pure-CLI offline variant** (`mework start --offline`
or `mework daemon start --offline` without `--with-mezon`). The
**server-stack offline variant** (`mework daemon start --offline --with-mezon`)
is defined in the `mezon-offline-bundle` capability and DOES spawn a local
`mework-server` and DOES enroll a runner against it; both variants share the
zero-external-infrastructure invariant (no Postgres, no Docker, no remote
services) but only the pure-CLI variant enforces zero-server / zero-enrollment.

#### Scenario: Start agent with no env vars configured

- **WHEN** user runs `mework start --workspace . --offline` with no `MEWORK_HUB_URL` or `DATABASE_URL`
- **THEN** the agent starts successfully without attempting any network connections

#### Scenario: Start agent with network unavailable

- **WHEN** user runs `mework start --workspace . --offline` with no network access
- **THEN** the agent starts and processes tasks without any network-related errors
