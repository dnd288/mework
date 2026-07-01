# c0010-session-workspaces: Evidence

## Gates

| Gate | Status | Notes |
|------|--------|-------|
| `go build ./...` | PASS | Clean build with Go 1.26.4 |
| `go vet ./...` | PASS | No vet warnings |
| `openspec validate` | TBD | Requires openspec CLI |

## What was implemented

- **shared/core/types.go**: Added `WorkspaceMode`, `SyncMode`, `BaseKind`, `BaseSpec`,
  `HookStage`, `HookResult`, `SyncResult`, `WorkspaceSpec` types; augmented `Workspace`
  with `Spec` and `Session` fields; added `Stage` field to `Hook`.

- **shared/grant/grant.go**: Added three workspace grant operations:
  `OpWorkspaceRead` ("workspace.read"), `OpWorkspaceWrite` ("workspace.write"),
  `OpWorkspacePush` ("workspace.push").

- **client/workspacefs/workspacefs.go**: Replaced the ObjectStore-stub with the
  full `WorkspaceFS` interface (ReadFile/WriteFile/List/Remove/Stat) and
  `LocalWorkspaceFS` implementation with path normalization, `..` traversal
  blocking, write confinement to a granted prefix, and shared-root read fallback.

- **server/storage/storage.go**: Added `WorkspaceStatus`, `WorkspaceSession`,
  and `WorkspaceManager` interface (Attach/Get/Detach/Sync/Status/
  MountSharedRoot/Publish/Bootstrap/RunHooks).

- **server/storage/manager.go** (new): Full `WorkspaceManager` implementation
  with session lifecycle management, object-store-backed sync, shared-root
  mounting, scoped publish, base materialization (stub), and lifecycle hook
  execution (stub — delegates to sandbox driver when available).
