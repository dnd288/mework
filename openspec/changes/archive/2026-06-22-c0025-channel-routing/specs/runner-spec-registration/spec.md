# Runner Spec Registration Specification

## Purpose

Define how runners declare the agent specs they support and how the system selects a runner based on spec compatibility. A runner that declares `specs: ["claude-code", "codex"]` is eligible for tasks requiring either spec; a runner with no specs declared is considered compatible with all specs for backward compatibility. Owned by `libs/server/registry/` and `libs/server/orchestrator/`.

## ADDED Requirements

### Requirement: Runner declares specs on enrollment

A runner SHALL declare the agent specs it can execute as part of the enrollment request. Specs SHALL be an array of strings matching agent names from the agent catalog (e.g. `claude-code`, `codex`). The enrollment endpoint SHALL validate each spec against the catalog and reject unknown specs.

#### Scenario: Enroll with specs

- **WHEN** a runner enrolls with `{"code":"my-runner", "specs":["claude-code", "codex"]}`
- **THEN** the runner is registered and its specs are stored in the `runtimes.specs` column

#### Scenario: Reject unknown spec

- **WHEN** a runner enrolls with `specs: ["non-existent-agent"]`
- **THEN** the enrollment is rejected with a 400 Bad Request

#### Scenario: Backward-compatible enrollment (no specs)

- **WHEN** a runner enrolls without a `specs` field
- **THEN** the runner is registered with `specs = NULL` and is considered capable of any spec

### Requirement: Spec-aware runner selection

The runner selector SHALL filter runners by spec compatibility when a spec is specified. A runner matches a spec when the spec is present in its `specs` array, or when `specs` is NULL/empty (backward-compatible). Among matching runners, the selector SHALL pick the one with the fewest active channel bindings.

#### Scenario: Select runner matching spec

- **WHEN** a dispatch requires spec `"claude-code"` and runners `A` (specs: `["claude-code"]`) and `B` (specs: `["codex"]`) are online
- **THEN** runner `A` is selected and runner `B` is not considered

#### Scenario: Backward-compatible runner matches any spec

- **WHEN** a dispatch requires spec `"claude-code"` and the only online runner has `specs = NULL`
- **THEN** that runner is selected (backward compatibility preserved)

#### Scenario: Load-balanced across matching runners

- **WHEN** runners `A` (1 active channel) and `B` (0 active channels) both match the spec
- **THEN** runner `B` is selected (fewest active bindings)

### Requirement: Spec heartbeat update

A runner SHALL report its current specs in its periodic heartbeat to the server. The server SHALL update `runtimes.specs` on each heartbeat, allowing a runner to dynamically add or remove capabilities without re-enrolling.

#### Scenario: Specs updated via heartbeat

- **WHEN** a runner heartbeats with `{"specs": ["claude-code"]}` after previously having `["claude-code", "codex"]`
- **THEN** the server updates the runner's specs to `["claude-code"]`
