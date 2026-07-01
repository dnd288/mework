## ADDED Requirements

### Requirement: Channel routing is opt-in and disabled by default

Channel routing (per-resource session auto-provisioning) SHALL be **disabled by default** and
enabled only by explicit configuration. When disabled, verified webhook events SHALL be handled
by the legacy pipeline (enqueue → claim → write-back) with no dependence on the channel
auto-provisioner. The feature flag SHALL be configurable via environment so an operator can opt
in (e.g. for end-to-end testing) without a code change.

#### Scenario: Default deployment uses the legacy pipeline

- **WHEN** the server starts without channel routing explicitly enabled
- **THEN** channel routing is off and verified webhooks are handled by the legacy
  enqueue/claim/write-back pipeline

#### Scenario: Channel routing can be enabled by configuration

- **WHEN** the channel-routing environment flag is set to enabled
- **THEN** the server activates channel routing for that deployment
