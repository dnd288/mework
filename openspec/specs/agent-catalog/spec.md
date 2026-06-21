# Agent Catalog Specification

## Purpose

Define the artifact storage and dispatch model for the agent hub: versioned,
immutable agent artifacts in type-agnostic forms, pull authorization, targeted
dispatch via the message bus, and scoped permission grants that constrain what
a dispatched agent may do. Owned by the agent catalog subsystem.

## Requirements

### Requirement: Versioned agent artifacts

The hub SHALL store **agents** as versioned, immutable artifacts. Each agent MUST
have a stable name and one or more immutable versions; publishing the same version
twice MUST NOT silently overwrite it. An agent version MUST be resolvable by
explicit version or by a moving pointer (e.g. `latest`).

#### Scenario: Publish a new agent version

- **WHEN** an operator publishes agent `code-fixer` version `1.2.0`
- **THEN** the hub stores it as an immutable version retrievable by `code-fixer@1.2.0`

#### Scenario: Republishing an existing version is rejected

- **WHEN** an operator publishes `code-fixer@1.2.0` and that version already exists with different content
- **THEN** the hub rejects the publish rather than overwriting the immutable version

#### Scenario: Resolve a moving pointer

- **WHEN** a client resolves `code-fixer@latest`
- **THEN** the hub returns the concrete version currently designated latest

### Requirement: Type-agnostic artifact form

The catalog SHALL support three artifact forms — a **definition/manifest** (prompt, workflow, declared needs), a **packaged/container image reference**, and a **bundle** (self-contained zip with folder structure) — and MUST record which form each version uses so a consumer can decide how to materialize it.

#### Scenario: Pull a definition-form agent

- **WHEN** a runner pulls an agent whose form is `definition`
- **THEN** the hub returns the manifest content and the form indicator so the runner materializes it as a definition

#### Scenario: Pull an image-form agent

- **WHEN** a runner pulls an agent whose form is `image`
- **THEN** the hub returns the image reference and the form indicator so the sandbox driver can pull/run the image

#### Scenario: Pull a bundle-form agent

- **WHEN** a runner pulls an agent whose form is `bundle`
- **THEN** the hub returns the zip bytes and the form indicator so the runner extracts and materializes the folder structure

### Requirement: Pull an agent

The hub SHALL expose an endpoint for an authorized runner to **pull** a resolved
agent version at dispatch time. The pull MUST be authorized against the puller's
identity and grant.

#### Scenario: Authorized pull succeeds

- **WHEN** an enrolled runner with a valid grant pulls a dispatched agent version
- **THEN** the hub returns the artifact (or its reference) for that version

#### Scenario: Unauthorized pull is denied

- **WHEN** a caller without a valid grant for an agent attempts to pull it
- **THEN** the hub denies the pull

### Requirement: Dispatch an agent to a target

The hub SHALL dispatch a resolved agent version to a target runner/session by
**publishing a dispatch message to that target's topic** (see the `message-bus`
capability). A dispatch MUST reference the exact agent version and carry the
permission grant for that run.

#### Scenario: Dispatch reaches the target runner

- **WHEN** an operator dispatches `code-fixer@1.2.0` to runner `R`
- **THEN** a dispatch message referencing `code-fixer@1.2.0` and its grant is published to runner `R`'s topic

### Requirement: Scoped permission grants

The hub SHALL attach a **scoped permission grant** to every dispatch, declaring the
operations the dispatched agent is permitted to perform (the "permitted
operations"). The grant MUST be explicit and least-privilege by default (no grant
means no privileged operation), and MUST travel with the dispatch so it can be
enforced downstream.

#### Scenario: Dispatch carries an explicit grant

- **WHEN** an agent is dispatched with a grant permitting only operations `X` and `Y`
- **THEN** the dispatch message carries a grant scoped to `X` and `Y` and nothing else

#### Scenario: Absent grant denies privileged operations

- **WHEN** an agent is dispatched without a grant for operation `Z`
- **THEN** operation `Z` is treated as not permitted for that run

#### Scenario: A tampered grant fails integrity verification

- **WHEN** a grant whose signature does not match its contents is presented for enforcement
- **THEN** verification fails and the run is denied, so a runner cannot widen its own scope

#### Scenario: Grants are scoped per run, not per identity

- **WHEN** the same runner is dispatched twice — once with a broad grant and once with a minimal grant
- **THEN** each run is bound to its own grant and the minimal run is restricted regardless of the broad run's privileges

### Requirement: Bundle-form sandbox artifact

The catalog SHALL support a third artifact form — `"bundle"` — in addition to the existing `"definition"` and `"image"` forms. A bundle SHALL be a zip file containing a standardized folder structure:
- `sandbox.yaml` (metadata: name, version, spec, backend, author)
- `definition.md` (agent prompt / system message)
- `tools/` (MCP tools and plugins)
- `hooks/` (lifecycle scripts)
- `assets/` (reference data)
- `config/` (default config overrides)

The zip SHALL be stored as `payload BYTEA`, exactly like the `"definition"` form stores its text payload — no schema change is needed.

#### Scenario: Publish a bundle-form sandbox

- **WHEN** a developer zips a local sandbox folder and publishes it with `form: "bundle"`
- **THEN** the catalog stores the zip as an immutable agent version

### Requirement: Catalog as sandbox registry

The agent catalog SHALL serve as the sandbox registry. Any machine SHALL be able to publish a sandbox definition (definition, image, or bundle form) and any authorized worker SHALL be able to pull and materialize it.

#### Scenario: Push and pull a bundle

- **WHEN** a developer publishes a bundle and another worker pulls it
- **THEN** the worker extracts the zip to an isolated workdir, reads `sandbox.yaml` and `definition.md`, and runs the sandbox

### Requirement: Dispatch with channel context

The `Dispatch` endpoint SHALL be extended to accept an optional `channel_key` parameter. When provided, the dispatch SHALL include the channel context (provider code, resource ID, spec) so the worker knows which channel to subscribe to.

#### Scenario: Dispatch with channel binding

- **WHEN** the auto-provisioner dispatches an agent with `channel_key: "mello:TICKET-99"` to a worker
- **THEN** the worker subscribes to `channel.mello.TICKET-99.*` after pulling and materializing the agent
