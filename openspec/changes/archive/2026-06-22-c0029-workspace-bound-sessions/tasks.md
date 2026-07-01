## 1. Workspace binding (core + engine) — landed

- [x] 1.1 Add `Workspace core.Workspace` to `core.RunSpec` (`libs/shared/core/types.go`) — additive, optional
- [x] 1.2 Local engine: use `spec.Workspace.Path` as the sandbox working directory when set, else current `SandboxID` behavior (`libs/sandbox/engine/local/runner.go`)
- [x] 1.3 Container engines: mount the bound workspace into the sandbox via `Sandbox.Mount` after start (manager/runtime path)

## 2. Thread workspace through the runner — landed

- [x] 2.1 Add a workspace field to `SessionDeps`/`RunDeps` and set `spec.Workspace` in `OpenSession` and `RunByReference`
- [x] 2.2 Keep stdin-not-argv and one-agent-per-sandbox invariants; unbound path unchanged

## 3. Catalog-backed definition resolver (client) — landed

- [x] 3.1 New `libs/client/catalog` package: `HTTPDefinitionResolver` implementing `runner.DefinitionResolver`
- [x] 3.2 Split `ref` on `@` (default `latest`), call `GET /api/v1/agents/{name}?version=<v>`, decode payload → `SandboxBundleMetadata`
- [x] 3.3 Map 404 → `runner.ErrDefinitionNotFound`

## 4. mework.yml workspace config + local resolver

- [x] 4.1 Define the `mework.yml` workspace config (decodes to `sandbox.SandboxBundleMetadata` + workspace settings); a loader that reads it from a workspace dir
- [x] 4.2 `FileDefinitionResolver` in `libs/client/catalog` implementing `runner.DefinitionResolver` by loading `mework.yml` from the workspace dir; missing file → `runner.ErrDefinitionNotFound`

## 5. Workspace pack / push / pull

- [x] 5.1 `pack`: archive a workspace dir (`mework.yml` at root + `.claude/` + files) into a bundle (zip)
- [x] 5.2 `push`: register the bundle into the catalog via `POST /api/v1/agents/{name}/versions` (form `bundle`), immutable per version
- [x] 5.3 `pull`: fetch a registered bundle and extract it into a local workspace directory
- [x] 5.4 Server: accept `mework.yml` as the bundle manifest in the catalog bundle validator (minimal; no schema migration)
- [x] 5.5 CLI: `mework workspace pack|push|pull` wired over the client functions

## 6. Two start modes

- [x] 6.1 Server start: resolve the registered config via `HTTPDefinitionResolver`, open a workspace-bound session, run on the client
- [x] 6.2 Local-direct start: start from local `mework.yml` via `FileDefinitionResolver` with a local `mework auth` grant; daemon starts the workspace as a local sandbox, no server

## 7. Read-back of workspace artifacts

- [x] 7.1 Use `client/workspacefs.NewLocal(workspaceDir, ...)` to list and read produced artifacts after a turn
- [x] 7.2 Demonstrate updating an artifact in the workspace and re-reading it

## 8. End-to-end example (examples/remote-claude)

- [x] 8.1 Workspace fixture: `mework.yml` (local engine + stub backend) + `.claude/settings.json`
- [x] 8.2 Deterministic **stub backend** (shell script): reads the task from stdin, writes an artifact into its CWD (the workspace)
- [x] 8.3 Server-mode flow: register via real `hub.NewServer`/`httptest` (Postgres-gated, skip without `TEST_DATABASE_URL`), resolve, open a workspace-bound session, `Send`, read events
- [x] 8.4 Local-direct flow: start from local `mework.yml` with no server; assert the artifact lands in the workspace
- [x] 8.5 pack → push → pull round-trip; assert the pulled workspace contains `mework.yml` + files
- [x] 8.6 Read the artifact back and update it via `workspacefs`
- [x] 8.7 Expand `examples/remote-claude/go.mod` with `libs/server` + `libs/client`; update README with the mework.yml + dual-start + pack/push/pull flow

## 9. Validation

- [x] 9.1 Table-driven unit tests: `FileDefinitionResolver` (load/missing), pack/push/pull round-trip, container `Mount` of a bound workspace
- [x] 9.2 `make vet` and `make test` green (server-mode example test skips without Postgres; local-direct test runs without a DB)
- [x] 9.3 `openspec validate c0029-workspace-bound-sessions --strict` passes
