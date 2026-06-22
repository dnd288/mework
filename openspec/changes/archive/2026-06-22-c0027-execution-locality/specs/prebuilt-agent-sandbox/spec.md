## ADDED Requirements

### Requirement: Runner-side execution; server is gateway and registry

The daemon and the sandbox SHALL run on the **runner** (the local agent machine), never
on the server. `mework-server` SHALL act only as a **gateway and registry**: webhook
ingress, the agent/definition **catalog (registry)**, session **metadata**, and the
**message-bus** control/stream topics. The server MUST NOT spawn a sandbox or execute an
agent process. A prebuilt definition MAY be published to the server registry, but it
SHALL be resolved, materialized, and executed on the runner, so source code and provider
credentials stay on the runner.

#### Scenario: Server stores the definition, runner executes it

- **WHEN** a prebuilt definition is published to the server and then run
- **THEN** the server stores it in the catalog/registry, and the runner resolves it and spawns the sandbox locally to execute the agent

#### Scenario: Server never spawns a sandbox

- **WHEN** a session is created and driven
- **THEN** the server holds only the session metadata and the bus topics, while the long-lived sandbox and agent process exist only on the runner

#### Scenario: Server code does not depend on the sandbox engine

- **WHEN** the server module is built
- **THEN** it does not import the sandbox engine or runtime packages, so it cannot start a sandbox
