## ADDED Requirements

### Requirement: Runner selection

The system SHALL **select a target runner** for each dispatch, load-balancing across
eligible online runners so no single runner is overloaded. A dispatch belonging to an
existing session MUST be routed back to that session's bound runner (affinity). When
no eligible runner exists, selection MUST fail clearly rather than silently drop the
dispatch.

#### Scenario: Dispatch load-balances across eligible runners

- **WHEN** multiple dispatches are placed for an agent with several eligible online runners
- **THEN** the selector spreads them across runners so no single runner is overloaded

#### Scenario: Session dispatches keep affinity to their runner

- **WHEN** a follow-up dispatch is selected for a session already bound to a runner
- **THEN** it is routed back to that same runner

#### Scenario: No eligible runner is handled gracefully

- **WHEN** a dispatch is attempted and no online runner is eligible for the agent
- **THEN** selection fails clearly (the dispatch is queued or rejected, not silently dropped)

### Requirement: Grant-scoped secret injection

The system SHALL inject secrets into a provisioned sandbox **bounded by the dispatch's
grant**: only secrets within the grant's scope may be injected, and an out-of-scope
secret MUST be refused. Injected secrets MUST be delivered out-of-band (environment
or file) and MUST NEVER appear in argv, command lines, or streamed logs.

#### Scenario: Grant-scoped secrets are injected into the sandbox

- **WHEN** a dispatch whose grant scopes a secret is provisioned and that secret is injected
- **THEN** the secret is made available inside that sandbox only

#### Scenario: Injected secrets never appear in argv or logs

- **WHEN** a secret is injected into a running sandbox and the agent runs and emits logs
- **THEN** the secret value is absent from argv, command lines, and streamed logs

#### Scenario: Secrets outside the grant are refused

- **WHEN** provisioning attempts to inject a secret outside the dispatch's grant scope
- **THEN** the injection is refused

#### Scenario: Scope enforcement is per-dispatch, not per-runner

- **WHEN** the same runner receives two dispatches with different grants
- **THEN** each dispatch receives only the secrets in its own grant
