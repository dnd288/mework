## Why

c0031 + c0033 (shipped) let the server create a session, dispatch an open-session message to
a runner, and have the daemon open a long-lived sandbox driven over the bus. But the daemon
always resolves the agent definition from the **server catalog** (`HTTPDefinitionResolver`)
and never binds the sandbox to a **local workspace directory**. For the
`mework sandbox start -w .` UX (c0037), we want exactly that: turn *this* local folder — its
`mework.yml` and files — into the running, server-addressable sandbox.

The building blocks exist: `core.RunSpec.Workspace` binds a dir, the local engine runs the
agent in it, `catalog.FileDefinitionResolver`/`LoadWorkspaceConfig` read a workspace's
`mework.yml`, and `SessionDeps.Workspace` plumbs a dir into `OpenSession`. What's missing is
carrying a **workspace path** from `POST /sessions` → the dispatch → the daemon, and having
the daemon resolve+bind from that path instead of the catalog.

## What Changes

- **Dispatch carries a workspace path.** Add `Workspace string` to `transport.Dispatch`
  (`libs/shared/transport/agent.go`). A non-empty `Workspace` means "open this session bound
  to that local directory."
- **Create accepts and forwards a workspace.** `createSessionRequest` gains
  `Workspace string`; `CreateSession` (`libs/server/session/handlers.go`) passes it to a
  widened `catalog.DispatchSessionToRunner(..., workspace string, g)`
  (`libs/server/catalog/dispatch.go`) which sets `Dispatch.Workspace`.
- **Daemon resolves from the workspace and binds it.** In
  `libs/client/runner/session_dispatch.go`, when `d.Workspace != ""`: build session deps with
  a **file** resolver (`catalog.FileDefinitionResolver{WorkspaceDir: d.Workspace}`, reading
  `<dir>/mework.yml`) and set `SessionDeps.Workspace = core.Workspace{Path: d.Workspace}` so
  `OpenSession` starts the sandbox bound to that dir. Otherwise the existing
  catalog/`HTTPDefinitionResolver` path is unchanged. The file resolver is supplied through a
  new injection seam `SetSessionWorkspaceResolverFactory` (mirroring the existing
  `SetSessionResolverFactory`, since the runner package cannot import the catalog package).
- **Same machine/user assumption.** The path is **absolute** and resolved on the daemon host;
  the daemon (same machine as the caller) reads `mework.yml` and binds the dir.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: a server-routed interactive session MAY be **bound to a local
  workspace directory** — the session-create request carries a workspace path that flows to
  the runner, which resolves the definition from that workspace's `mework.yml` and binds the
  sandbox to the directory.
- `daemon-runtime`: an open-session dispatch carrying a workspace path SHALL resolve the
  definition from that workspace (`mework.yml`) and bind the long-lived sandbox to the
  directory, rather than resolving from the server catalog.

## Impact

- **Shared:** `libs/shared/transport/agent.go` — `Dispatch.Workspace` (additive, optional).
- **Server:** `libs/server/catalog/dispatch.go` (`DispatchSessionToRunner` gains `workspace`);
  `libs/server/session/handlers.go` (`createSessionRequest.Workspace`, thread through).
- **Client:** `libs/client/runner/session_dispatch.go` (workspace branch + new
  `SetSessionWorkspaceResolverFactory` seam); daemon entrypoint wires the file-resolver
  factory.
- **Reuses** `catalog.FileDefinitionResolver` + `LoadWorkspaceConfig`
  (`libs/client/catalog/file_resolver.go`), `SessionDeps.Workspace`, `RunSpec.Workspace`.
- **Depends on** c0031 + c0033 (shipped). **Precedes** c0037 (the `sandbox` CLI that sends the
  workspace path). Backward-compatible: empty `Workspace` → existing catalog-resolved path.
- Preserves stdin-not-argv, one-agent-per-sandbox, and the c0027 boundary (sandbox runs on the
  runner). No schema migration.
