# Prebuilt Agent Sandbox Specification

## Purpose

Define a runnable agent as a **prebuilt sandbox definition** — a named, versioned,
immutable artifact binding an engine, an agent backend, an image/config, and resource
limits into a ready-to-run combo — and the execution model for running such a
definition either one-shot by reference or as a long-lived interactive multi-turn
session over a pluggable engine, with live observability and tenant-scoped,
grant-enforced remote control.
## Requirements
### Requirement: Prebuilt sandbox definition

The system SHALL represent a runnable agent as a **prebuilt sandbox definition**: a
named, versioned, **immutable** artifact that binds an **engine**, an **agent
backend**, an **image/config**, and **resource limits** into a ready-to-run combo.
A definition MUST be resolvable by explicit reference (`name@version`) and by a moving
pointer (e.g. `latest`). Republishing an existing version with different content MUST
be rejected rather than silently overwritten. Definitions SHALL be stored as
agent-catalog artifacts (reusing the sandbox bundle metadata), so no new storage
system or schema migration is introduced to add one.

#### Scenario: Publish a prebuilt definition

- **WHEN** an operator publishes definition `local-claude` version `1.0.0` binding engine `local` and backend `claude`
- **THEN** the system stores it as an immutable version retrievable by `local-claude@1.0.0`

#### Scenario: Republishing an existing version is rejected

- **WHEN** an operator publishes `local-claude@1.0.0` and that version already exists with different content
- **THEN** the system rejects the publish rather than overwriting the immutable version

#### Scenario: Resolve a moving pointer

- **WHEN** a client resolves `local-claude@latest`
- **THEN** the system returns the concrete version currently designated latest

### Requirement: Sandbox is the agent runner with a pluggable engine

The system SHALL treat the **sandbox as the agent runner**, where the **engine** is a
property of the sandbox selected by the definition. The `local` engine SHALL be the
default. Each sandbox MUST run **exactly one agent** (one agent per sandbox). Adding a
new engine or a new agent backend MUST NOT require a schema migration.

#### Scenario: Definition selects the engine

- **WHEN** a definition declares engine `docker`
- **THEN** the system materializes the sandbox using the `docker` engine, and a definition declaring `local` uses the `local` engine

#### Scenario: Default engine

- **WHEN** a definition omits an engine
- **THEN** the system uses the `local` engine

#### Scenario: One agent per sandbox

- **WHEN** a sandbox is already running an agent and another agent start is requested for the same sandbox
- **THEN** the system rejects the second start

#### Scenario: New backend without migration

- **WHEN** a definition references a new backend such as `windows-claude` or `v0`
- **THEN** the system runs it without requiring a database migration

### Requirement: Run a prebuilt sandbox by reference

The system SHALL run a prebuilt definition **by reference**: resolve the definition,
start a sandbox via its engine, and run the agent. Prompt and turn content MUST be fed
to the backend over **stdin, never as a command-line argument**.

#### Scenario: Resolve and run

- **WHEN** a caller runs `local-claude@1.0.0` with an instruction
- **THEN** the system resolves the definition, starts a `local` sandbox, and runs `claude` against the instruction

#### Scenario: Content is not placed on the command line

- **WHEN** a run is started with attacker-controllable content
- **THEN** the content is written to the backend's stdin and never appears in argv

#### Scenario: Unknown reference is rejected

- **WHEN** a caller runs a reference that resolves to no definition version
- **THEN** the system rejects the run with a not-found result rather than starting a sandbox

### Requirement: Interactive multi-turn session over a long-lived sandbox

The system SHALL support an **interactive session** in which a single sandbox stays
**alive across multiple turns**. Opening a session from a definition MUST start the
sandbox once; each subsequent turn MUST be delivered to the running agent over stdin;
the session MUST support **cancel/interrupt** of an in-flight turn without destroying
the session; and the session MUST have an explicit lifecycle (create, attach, close)
with **idle reaping**. Sessions MUST be **owned** and **tenant-scoped**.

#### Scenario: Multi-turn over one sandbox

- **WHEN** a user opens a session from `local-claude@1.0.0` and sends two turns
- **THEN** both turns are handled by the same long-lived sandbox process, the second after the first

#### Scenario: Cancel an in-flight turn

- **WHEN** a user cancels while a turn is running
- **THEN** the in-flight turn is interrupted and the session remains open for further turns

#### Scenario: Idle session is reaped

- **WHEN** a session has been idle past its idle timeout
- **THEN** the system closes the session and destroys its sandbox

#### Scenario: Ownership is enforced

- **WHEN** a caller who is not the session owner attempts to attach to or send a turn into the session
- **THEN** the system denies the request

### Requirement: Live logs and observability

The system SHALL stream **live events** from a session/run — at least
`token`, `message`, `done`, and `error` — to subscribers, with exactly one terminal
event (`done` or `error`) per turn. A subscriber that attaches late MUST receive a
**tail of recent events then the live stream** (tail-then-live). Session **status**
and a tenant-scoped **list** of sessions MUST be queryable.

#### Scenario: Live token stream

- **WHEN** a subscriber is attached while a turn runs
- **THEN** it receives `token`/`message` events and one terminal `done` (or `error`) for that turn

#### Scenario: Late subscriber gets tail-then-live

- **WHEN** a subscriber attaches after a turn has already started
- **THEN** it receives recent buffered events and then continues with the live stream

#### Scenario: Query status and list

- **WHEN** an operator queries a session's status and lists sessions for the tenant
- **THEN** the system returns the session's current status and only that tenant's sessions

### Requirement: Pre-baked image for container engines

For **container engines** (e.g. `docker`), a definition SHALL pin a **pre-baked
image** that already contains the agent CLI, so the engine performs **no install step**
at run time. Engines without an image concept (e.g. `local`) MUST ignore the image
field.

#### Scenario: Container engine uses the pinned image

- **WHEN** a `docker` definition pins image `mework/claude:1.0.0`
- **THEN** the sandbox runs that image and installs nothing at run time

#### Scenario: Local engine ignores image

- **WHEN** a `local` definition is run
- **THEN** the system runs the host backend and ignores any image field

### Requirement: Remote-control authorization

All session operations (create, attach, send turn, cancel, close, list) SHALL be
authorized: **tenant-isolated**, bound to the session **owner**, and **grant-enforced**
per the `auth-and-secrets` permission model. A caller lacking the required grant or
crossing a tenant boundary MUST be denied. For listing, the tenant scope MUST be derived
from the authenticated caller, never from a caller-supplied argument.

#### Scenario: Cross-tenant access denied

- **WHEN** a caller in tenant `A` attempts to operate on a session in tenant `B`
- **THEN** the system denies the operation

#### Scenario: Operation without grant denied

- **WHEN** a caller attempts a session operation for which it has no permission grant
- **THEN** the system denies the operation

#### Scenario: List is scoped to the caller's own tenant

- **WHEN** a caller lists sessions
- **THEN** the system returns only the authenticated caller's tenant's sessions, regardless of any tenant argument the caller supplies

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

### Requirement: Server-routed chat transport

The server SHALL provide HTTP transport for an interactive chat over a session, acting as a
**thin bus relay that holds no conversation state**. An authenticated, owning caller SHALL
be able to **submit a chat turn** for a session and **stream the session's events** back.
The runner SHALL be able to **deliver per-turn events** to the server for relay. Submitting
a turn publishes it to the session's input topic; streaming subscribes to the session's
control topic; runner-delivered events are republished on the control topic. Conversation
state and the sandbox remain on the runner.

#### Scenario: Submit a chat turn

- **WHEN** an owning caller issues `POST /api/v1/sessions/{id}/messages` with a chat message
- **THEN** the server publishes the turn to the session's input topic for the runner and
  accepts the request, storing no conversation state

#### Scenario: Stream session events

- **WHEN** an owning caller issues `GET /api/v1/sessions/{id}/stream`
- **THEN** the server streams the session's `token`/`message`/`done`/`error` events as they
  arrive on the session's control topic

#### Scenario: Non-owner cannot submit or stream

- **WHEN** a caller who does not own the session attempts to submit a turn or stream its
  events
- **THEN** the server denies the request

#### Scenario: Runner delivers events for relay

- **WHEN** the runner posts a session event to `POST /api/v1/runners/sessions/{id}/events`
  with its runtime credential
- **THEN** the server republishes it on the session's control topic so subscribers receive
  it

### Requirement: Workspace-bound server-routed session

A server-routed interactive session SHALL support being **bound to a local workspace
directory**. The session-create request MAY carry a workspace path; when present, the server
SHALL forward that path to the runner on the open-session dispatch, and the runner SHALL
resolve the agent definition from that workspace's `mework.yml` and bind the long-lived
sandbox to the directory. The server SHALL remain a relay per the runner-side-execution boundary: it
forwards the workspace path and never reads the workspace itself. When no workspace path is
provided, the session resolves its definition from the server catalog as before.

#### Scenario: Create a workspace-bound session

- **WHEN** a caller creates a session with a workspace path
- **THEN** the server dispatches an open-session message carrying that path to the runner,
  and the runner opens the sandbox bound to that directory using the workspace's `mework.yml`

#### Scenario: No workspace falls back to the catalog

- **WHEN** a caller creates a session without a workspace path
- **THEN** the runner resolves the definition from the server catalog, unchanged

#### Scenario: Server does not read the workspace

- **WHEN** a workspace-bound session is created
- **THEN** the server only forwards the workspace path; the workspace files are read solely
  on the runner

