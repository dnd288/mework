## Context

c0026 built prebuilt definitions + interactive sessions; c0027 fixed the boundary
(runner runs the agent; server = gateway + registry). This change makes a **workspace**
the portable unit: a dir with `mework.yml` + `.claude/`, registrable and runnable two
ways. Already landed here: `RunSpec.Workspace`, local-engine workdir binding, runner
threading, and the catalog `HTTPDefinitionResolver`. Remaining: the `mework.yml`
convention + file resolver, workspace pack/push/pull, the local-direct start path, and an
example proving both start modes.

## Goals / Non-Goals

**Goals:**
- Name the workspace config `mework.yml`; load it locally and resolve it from the server.
- Run a session/run inside a workspace dir; artifacts are inspectable/updatable.
- Pack a workspace to a bundle, push it to the server, pull it back.
- Start a workspace **two ways**: pulled-from-server, and **fully local** (no server).
- An example demonstrating both modes + a pack→push→pull round-trip + artifact read/update.

**Non-Goals:**
- A server-side session-create/dispatch HTTP API (sessions are opened on the runner;
  full client→server→runner dispatch stays future work).
- A real object-store ArtifactStore (still dummy) — artifacts are read from the workspace.
- Remote workspace **sync** (continuous push/pull of live edits) and base materialization
  (git/archive/store) — out of scope; pack/push/pull is a whole-workspace transfer.

## Decisions

- **`mework.yml` is the workspace config**, replacing the `sandbox.yaml` name in this
  feature's surface. It carries the prebuilt-definition fields (engine/backend/image) and
  decodes to `sandbox.SandboxBundleMetadata`; existing in-tree `definitions/*/sandbox.yaml`
  are left as-is (no global rename) to keep the change minimal. A workspace is identified
  by its `mework.yml`.
- **Two resolvers, one runner contract.** Both implement `runner.DefinitionResolver`:
  - `HTTPDefinitionResolver` *(landed)* — server path, resolves over the catalog.
  - `FileDefinitionResolver` *(new)* — local path, loads `mework.yml` from the workspace
    dir. The local-direct start uses this with **no server**.
- **Two start modes share the workspace-bound session.** Both call `OpenSession` with
  `SessionDeps.Workspace` set to the workspace dir; they differ only in resolver +
  identity:
  - **Server:** `HTTPDefinitionResolver` + a grant obtained against the server.
  - **Local-direct:** `FileDefinitionResolver` + a **local grant from `mework auth`**
    (an `OpSpawn` grant minted locally), driven by `mework daemon`. The agent runs as a
    **local sandbox** entirely on the client.
- **pack/push/pull = whole-workspace transfer over the catalog bundle form.** `pack` zips
  the workspace (`mework.yml` at root + `.claude/` + files); `push` POSTs it as form
  `bundle` to `POST /api/v1/agents/{name}/versions`; `pull` GETs the version and extracts
  the zip into a local dir. The server's bundle validation is updated minimally to accept
  `mework.yml` as the manifest (reusing the existing `bundle` form + storage); extraction
  is client-side. Registration stays immutable-per-version.
- **CLI surface:** `mework workspace pack|push|pull` (thin wrappers over the client
  functions), so the flow is usable outside the example.
- **`.claude/` travels with the workspace**, not the registered definition — it is read by
  the agent from its working directory (the bound workspace), matching how Claude Code
  reads project settings.
- **Stub backend** = a shell script reading the task from **stdin** and writing a
  deterministic artifact into its CWD (the workspace) — CI-safe, proves stdin-not-argv +
  workspace-write, and the same fixture works in both start modes.

## Risks / Trade-offs

- [Server bundle validation change] → keep it minimal: accept `mework.yml` as the manifest
  for the `bundle` form; do not require `definition.md`. Scope the change to the catalog
  bundle validator; no schema migration.
- [Local engine has no isolation] → workspace binding gives direct host FS access;
  documented trusted-only (unchanged). Container engines isolate via `Mount`.
- [Two start modes = more surface] → both funnel through one `OpenSession`/workspace-bound
  path; only resolver + grant source differ, limiting duplication.
- [Example needs Postgres] → only the server-mode test; the local-direct test runs with no
  DB. Both skip/▸run independently so `make test` stays green without Postgres.

## Migration Plan

Additive. `RunSpec.Workspace` is optional (landed, backward-compatible). `mework.yml` is a
new convention; existing `sandbox.yaml` definitions keep working. New resolver, CLI, and
example are net-new.
