# Runner Spec Registration Specification

## Purpose

Define how runners declare the agent specs they support and how the system selects a runner based on spec compatibility. A runner that declares `specs: ["claude-code", "codex"]` is eligible for tasks requiring either spec; a runner with no specs declared is considered compatible with all specs for backward compatibility. Owned by `libs/server/registry/` and `libs/server/orchestrator/`.

## Requirements

### Requirement: Runner declares specs on enrollment

A runner SHALL declare the agent specs it can execute as part of enrollment. Specs SHALL be an array of strings matching agent names from the agent catalog. The enrollment endpoint SHALL validate each spec against the catalog and reject unknown specs.

#### Scenario: Enroll with specs

- **WHEN** a runner enrolls with `{"code":"my-runner", "specs":["claude-code", "codex"]}`
- **THEN** the runner is registered and its specs are stored

#### Scenario: Backward-compatible enrollment (no specs)

- **WHEN** a runner enrolls without a `specs` field
- **THEN** the runner is registered with `specs = NULL` and is considered capable of any spec

### Requirement: Spec-aware runner selection

The runner selector SHALL filter runners by spec compatibility. A runner matches a spec when the spec is present in its `specs` array, or when `specs` is NULL. Among matching runners, the selector SHALL pick the one with the fewest active channel bindings.

#### Scenario: Select runner matching spec

- **WHEN** a dispatch requires spec `"claude-code"` and runners A (`["claude-code"]`) and B (`["codex"]`) are online
- **THEN** runner A is selected

#### Scenario: Backward-compatible runner matches any spec

- **WHEN** a dispatch requires spec `"claude-code"` and the only online runner has `specs = NULL`
- **THEN** that runner is selected

### Requirement: Spec heartbeat update

A runner SHALL report its current specs in its periodic heartbeat. The server SHALL update `runtimes.specs` on each heartbeat.

#### Scenario: Specs updated via heartbeat

- **WHEN** a runner heartbeats with `{"specs": ["claude-code"]}` after previously having `["claude-code", "codex"]`
- **THEN** the server updates the runner's specs
