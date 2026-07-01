## 1. Dispatch + create carry a workspace (TDD)

- [x] 1.1 Write a test (fail first): `transport.Dispatch` round-trips `Workspace` through JSON.
- [x] 1.2 Add `Workspace string \`json:"workspace,omitempty"\`` to `transport.Dispatch`
      (`libs/shared/transport/agent.go`).
- [x] 1.3 Write a server test (fail first): `POST /sessions` with a `workspace` field forwards
      it to the dispatcher.
- [x] 1.4 Widen `catalog.DispatchSessionToRunner(..., workspace string, g)` to set
      `msg.Workspace`; add `Workspace` to `createSessionRequest` and thread it through
      `CreateSession` (`libs/server/session/handlers.go`). Update the existing call site.

## 2. Daemon resolves + binds the workspace (TDD)

- [x] 2.1 Write `session_dispatch_test.go` (fail first): a dispatch with `Workspace` set opens
      the session via a file resolver and binds `RunSpec.Workspace.Path` to that dir; turns
      still route to `Session.Send`; empty `Workspace` keeps the catalog path.
- [x] 2.2 Add `SetSessionWorkspaceResolverFactory` + `sessionWorkspaceResolverFor` var
      (mirror `SetSessionResolverFactory`). In `processSessionDispatch`, when
      `d.Workspace != ""`, use the workspace resolver and set
      `deps.Workspace = core.Workspace{Path: d.Workspace}`.
- [x] 2.3 Wire the daemon entrypoint to inject
      `func(p) DefinitionResolver { return &catalog.FileDefinitionResolver{WorkspaceDir: p} }`.

## 3. Validation

- [x] 3.1 `make vet` + `make test ./libs/server/... ./libs/client/... ./libs/shared/...` green;
      new tests fail-first then pass.
- [x] 3.2 `make test` (full) green.
- [x] 3.3 Smoke (with c0035 + a running daemon): `POST /sessions` with a `workspace` path opens
      a local sandbox bound to that dir; a turn runs against the workspace files.
