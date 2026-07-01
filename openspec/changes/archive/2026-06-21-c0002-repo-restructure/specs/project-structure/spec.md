## ADDED Requirements

### Requirement: Multi-module workspace

The repository SHALL be organized as a `go.work` workspace of four Go modules — three
components (`client`, `server`, `sandbox`) and one leaf contract module (`shared`) —
each with its own `go.mod`. Binaries SHALL wire exactly one runnable component
(`cmd/mework` = client, `cmd/mework-server` = server, optional `cmd/mework-sandbox` =
standalone sandbox host) and contain no domain logic beyond wiring.

#### Scenario: Each module is independently buildable

- **WHEN** a module's build/test target is run
- **THEN** that module builds and tests on its own using `go.work` to resolve its sibling dependencies

#### Scenario: A package has an unambiguous home

- **WHEN** a contributor adds a new package
- **THEN** it belongs to exactly one module (`shared`, `server`, `client`, or `sandbox`) per the documented ownership rules

#### Scenario: Binaries only wire

- **WHEN** the `cmd/mework`, `cmd/mework-server`, or `cmd/mework-sandbox` binary is built
- **THEN** its `main` package composes domain packages and holds no business logic of its own

### Requirement: Module dependency boundaries

Dependencies SHALL flow along a fixed module DAG: `shared` is the leaf and imports no
other module; `server` MAY import `shared`; `sandbox` MAY import `shared`; `client` MAY
import `shared` and `sandbox`. **`client` and `server` MUST NOT import each other, the
`server` MUST NOT import `sandbox`, and `shared` MUST NOT import any other module.**

#### Scenario: Client and server stay decoupled

- **WHEN** the module dependency graph is analyzed
- **THEN** no package in `client` imports `server` and no package in `server` imports `client`

#### Scenario: Server stays free of client and sandbox

- **WHEN** the module dependency graph is analyzed
- **THEN** no package in `server` imports `client` or `sandbox`

#### Scenario: Shared stays leaf-level

- **WHEN** the module dependency graph is analyzed
- **THEN** `shared` imports none of `client`, `server`, or `sandbox` and pulls in no heavy third-party dependency

#### Scenario: Violations are caught automatically

- **WHEN** a change introduces an import that breaks the module DAG
- **THEN** a CI check (import-guard lint) fails the build

### Requirement: Independent module build and test

Each module SHALL build and test **without** the other components' packages, and the
build SHALL expose per-module targets so each module's suite runs independently.

#### Scenario: Server builds without client or sandbox

- **WHEN** CI runs the server build/test target
- **THEN** it compiles and tests `server` (and `shared`) without requiring any `client` or `sandbox` package

#### Scenario: Client builds without server

- **WHEN** CI runs the client build/test target
- **THEN** it compiles and tests `client` (with `shared` and `sandbox`) without requiring any `server` package

### Requirement: Shared transport contract

The system SHALL keep the wire contracts — the client↔server SSE event schema and API
DTOs, and the runner↔sandbox protocol — in a single `shared/transport` package, and the
pluggable interfaces in a single `shared/ports` package, so each contract has one source
of truth that all modules depend on.

#### Scenario: One source of truth for the wire format

- **WHEN** an SSE event shape, an API DTO, or the runner↔sandbox protocol changes
- **THEN** it is edited once in `shared/transport` and every module picks up the change

#### Scenario: One source of truth for the plug points

- **WHEN** a pluggable interface (a driver/engine port) changes
- **THEN** it is edited once in `shared/ports` and consumers and adapters pick up the change

### Requirement: Pluggable driver and engine architecture

Every swappable backend SHALL be defined as an interface (port) in `shared/ports` and
implemented by adapters that register with a driver registry, so a new backend is added by
implementing the port and registering it without editing the consumers or the core.
Swappable backends include sandbox engines, object storage, message-bus brokers, provider
adapters, agent backends, and notifiers.

#### Scenario: Add a backend without touching consumers

- **WHEN** a new backend (e.g. a `cloudflare` sandbox engine or an `r2` object store) is added
- **THEN** it implements the relevant port and registers itself, and no consumer or core package is modified

#### Scenario: Consumers depend on the port, not the driver

- **WHEN** a consumer uses a backend
- **THEN** it imports the port from `shared/ports`, never a concrete driver package

#### Scenario: An out-of-tree engine plugs in via the shared contract

- **WHEN** a custom sandbox engine is shipped in an external module
- **THEN** it depends only on `mework/shared`, implements `SandboxDriver`/`Sandbox`, and is selected by wiring it into a binary

#### Scenario: The sandbox engine is chosen by capability and policy

- **WHEN** a run is dispatched
- **THEN** the engine is selected from each driver's advertised `SandboxCaps` and the dispatch grant (e.g. untrusted → an isolating engine), not hardcoded

### Requirement: Per-engine dependency isolation

Each engine or driver SHALL isolate its third-party dependency in its own subpackage, and
a binary SHALL link only the drivers it explicitly wires in, so components never carry
dependencies they do not use.

#### Scenario: A local-only client links no heavy engine SDK

- **WHEN** the `mework` client is built wiring only the local sandbox engine
- **THEN** the resulting binary links neither the docker SDK, the S3 SDK, nor the NATS client

#### Scenario: Selecting a driver is a one-line wiring change

- **WHEN** a binary needs to ship an additional engine/driver
- **THEN** it adds a single blank import for that driver and no other package changes

### Requirement: Repository split readiness

Each module SHALL depend only outward toward `shared`, so any module — or any
engine/driver subpackage — can become its own Git repository depending only on the
published `shared` contract, with `go.work` tying them locally during development.

#### Scenario: A module can move to its own repo unchanged

- **WHEN** a module (`client`, `server`, `sandbox`) or an engine/driver is extracted to a separate repository
- **THEN** it builds against the published `mework/shared` module with no code change beyond its `go.mod`/replace directives

### Requirement: Behavior-preserving migration

The restructure SHALL be behavior-preserving: the produced binaries (`mework`,
`mework-server`), their CLI surface, and their runtime behavior MUST be unchanged, and
the existing test suite MUST pass without modification to test assertions (only import
paths move and the unused `mcp-go` dependency is dropped).

#### Scenario: Binaries and behavior unchanged

- **WHEN** the restructure is applied and the workspace is built and tested
- **THEN** the same `mework` and `mework-server` binaries are produced and the full test suite passes with only import-path edits
