## ADDED Requirements

### Requirement: Resolve a registered definition from the server catalog

The client/runner SHALL resolve a prebuilt definition **by reference** against the
**server catalog** over HTTP. Given a reference of the form `name@version` (with a
missing version defaulting to the `latest` pointer), it MUST request the registered
definition from the server, decode it into the definition metadata, and use it to run.
An unknown reference MUST surface as a not-found error rather than a partial run.

#### Scenario: Resolve a published definition

- **WHEN** a definition has been registered to the server and a caller resolves `name@version`
- **THEN** the client retrieves that definition's metadata (engine, backend, image) from the server catalog

#### Scenario: Default to latest

- **WHEN** a caller resolves a reference with no explicit version
- **THEN** the client resolves the definition's `latest` pointer

#### Scenario: Unknown reference is not found

- **WHEN** a caller resolves a reference that the server has no version for
- **THEN** the client returns a not-found error and does not start a sandbox

### Requirement: Workspace config is mework.yml

A workspace SHALL be configured by a file named **`mework.yml`** at the workspace root,
declaring the prebuilt-definition fields (engine, backend, image) plus workspace
settings. The system SHALL load `mework.yml` from a local workspace directory and resolve
it to the same definition metadata used for a server-resolved reference, so a workspace
can run from its local config with no server.

#### Scenario: Load a local workspace config

- **WHEN** a workspace directory contains a `mework.yml`
- **THEN** the system loads it and resolves the definition (engine, backend, image) from it without contacting a server

#### Scenario: Missing config is reported

- **WHEN** a workspace directory has no `mework.yml`
- **THEN** the system returns a not-found error rather than starting a sandbox

### Requirement: Workspace-bound session

A session or run MAY be **bound to a workspace directory**. When bound, the agent SHALL
execute with that workspace as its working context: for the `local` engine the workspace
directory is the sandbox working directory (the agent reads and writes it directly); for
container engines the workspace is mounted into the sandbox via the mount seam. Files the
agent writes into the workspace SHALL persist and be **readable back** after the turn, so
the user can inspect and update the resulting artifacts. When no workspace is bound,
behavior is unchanged.

#### Scenario: Agent works in the bound workspace

- **WHEN** a session is opened bound to a workspace directory and the agent is asked to produce a file
- **THEN** the file is written into that workspace directory and remains there after the turn

#### Scenario: Artifacts are readable back

- **WHEN** the agent has written outputs into the bound workspace
- **THEN** the workspace can be listed and the produced artifacts read back (and updated) by the user

#### Scenario: Unbound run is unchanged

- **WHEN** a run is started with no workspace bound
- **THEN** it behaves exactly as a run without workspace binding (no regression)

### Requirement: Pack, push, and pull a workspace

A workspace SHALL be portable to and from the server: **pack** archives the workspace
(its `mework.yml` and files) into a bundle; **push** registers that bundle into the
server catalog under a name and immutable version; **pull** fetches a registered bundle
and extracts it into a local workspace directory. A re-pushed existing version MUST be
rejected (immutability), consistent with prebuilt definitions.

#### Scenario: Pack then push

- **WHEN** a user packs a workspace and pushes it as `name@version`
- **THEN** the server stores the bundle as an immutable version retrievable by that reference

#### Scenario: Pull recreates the workspace

- **WHEN** a user pulls a pushed `name@version`
- **THEN** the workspace's `mework.yml` and files are extracted into a local directory ready to run

### Requirement: Two start modes — server and local-direct

A workspace SHALL be startable **two ways**, both running the agent as a sandbox **on the
client**: (1) **server** — resolve the registered config from the server, then open a
workspace-bound session; (2) **local-direct** — start from the local `mework.yml` with
**no server**, where the daemon starts the workspace as a local sandbox and a locally
obtained identity/grant authorizes the run. Both modes MUST honor stdin-not-argv and
one-agent-per-sandbox, and neither runs the agent on the server.

#### Scenario: Start from the server

- **WHEN** a user starts a workspace whose config is registered on the server
- **THEN** the client resolves the config from the server and runs the agent in the bound workspace on the client

#### Scenario: Start fully locally

- **WHEN** a user starts a workspace from its local `mework.yml` with no server available
- **THEN** the daemon starts the workspace as a local sandbox using a local grant and runs the agent in the workspace, contacting no server
