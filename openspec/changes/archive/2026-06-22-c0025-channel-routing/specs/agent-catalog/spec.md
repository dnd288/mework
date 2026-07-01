# Agent Catalog Specification — Delta

## ADDED Requirements

### Requirement: Bundle-form sandbox artifact

The catalog SHALL support a third artifact form — `"bundle"` — in addition to the existing `"definition"` and `"image"` forms. A bundle SHALL be a zip file containing a standardized folder structure:

- `sandbox.yaml` (metadata: name, version, spec, backend, author)
- `definition.md` (agent prompt / system message)
- `tools/` (MCP tools and plugins, each in its own subdirectory with manifest)
- `hooks/` (lifecycle scripts: setup, teardown, pre-run, post-run)
- `assets/` (reference data, images, templates)
- `config/` (default config overrides: env vars, resource limits)

The zip SHALL be stored as `payload BYTEA` in `agent_versions`, exactly like the existing `"definition"` form stores its text payload — no schema change is needed, only the `form` column value `"bundle"`.

#### Scenario: Publish a bundle-form sandbox

- **WHEN** a developer zips a local sandbox folder and publishes it with `form: "bundle"` and the zip bytes as payload
- **THEN** the catalog stores the zip as an immutable agent version

#### Scenario: Pull a bundle-form sandbox

- **WHEN** a worker pulls agent version `code-reviewer@2.0.0` whose form is `bundle`
- **THEN** the server returns the zip bytes and the worker extracts the folder structure locally

#### Scenario: Bundle contains required metadata

- **WHEN** a bundle is published without `sandbox.yaml`
- **THEN** the server SHALL reject the publish with a clear error

### Requirement: Catalog as sandbox registry

The agent catalog SHALL serve as the sandbox registry. Any machine SHALL be able to publish a sandbox definition (definition, image, or bundle form) and any authorized worker SHALL be able to pull and materialize it. No separate sandbox store is needed.

#### Scenario: Push sandbox definition

- **WHEN** a developer publishes an agent version with form `"definition"` and a prompt payload
- **THEN** the sandbox definition is stored in the agent catalog and pullable by any authorized worker

#### Scenario: Push and pull a bundle

- **WHEN** a developer publishes a bundle and another worker pulls it
- **THEN** the worker extracts the zip to an isolated workdir, reads `sandbox.yaml` and `definition.md`, and runs the sandbox

### Requirement: Dispatch with channel context

The `Dispatch` endpoint SHALL be extended to accept an optional `channel_key` parameter alongside the existing `target`. When `channel_key` is provided, the dispatch SHALL include the channel context (provider code, resource ID, spec) in the dispatch payload so the worker knows which channel to subscribe to.

#### Scenario: Dispatch with channel binding

- **WHEN** the auto-provisioner dispatches an agent with `channel_key: "mello:TICKET-99"` to a worker
- **THEN** the worker subscribes to `channel.mello.TICKET-99.*` after pulling and materializing the agent

## MODIFIED Requirements

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
