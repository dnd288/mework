## Why

A session running an agent needs a real working directory that **survives across
sandbox runs** and can be **shared and resumed**. Today execution happens in an
ephemeral per-run host directory that vanishes when the sandbox is destroyed:
nothing persists the agent's working files, nothing lets a second sandbox pick the
work back up, and there is no controlled way to share outputs between sessions.

This change introduces **session/run-scoped workspaces** backed by the object
store. A folder is attached to a session, mounted read-write into the sandbox, and
**synced to remote (S3-compatible)** under the session's prefix so the work is
durable. Alongside it, a **shared root** is mounted read-only so the agent can read
across all published folders, while it may **push only the one grant-allowed
folder**. A workspace can also carry **base code** (clone a git repo, unpack an
archive, or copy a store template) and **lifecycle hooks** (init / pre_run /
post_run / pre_sync / post_sync) that the sandbox runs around the agent.

The agent never holds raw store credentials: sync goes through the object store's
presigned / hub-proxied path established by `c0008-object-storage`.

## What Changes

- A new **workspaces** capability composed of three module homes:
  - `client/workspacefs` — the agent-facing `WorkspaceFS` (Read/Write/List/Remove/
    Stat) with read-across-the-shared-root, write-confined-to-the-grant-prefix, and
    path-traversal blocked.
  - `server/storage` — the `WorkspaceManager` (Attach/Get/Detach/Sync/Status/
    MountSharedRoot/Publish/Bootstrap/RunHooks) wiring sync to the `ObjectStore`,
    managing the shared root, and publishing the one allowed folder.
  - `sandbox` runtime — provides Mount (rw workspace + ro shared root), Bootstrap
    (materialize `BaseSpec` then run init hooks), and RunHooks (drive lifecycle
    stages), feeding hook scripts over **stdin, never argv**.
- `WorkspaceSpec` (mount path, remote prefix, mode, sync mode, shared roots, base,
  hooks), `BaseSpec` (`git` | `archive` | `store`), and `Hook` / `HookStage`
  (`init` / `pre_run` / `post_run` / `pre_sync` / `post_sync`).
- A `SyncMode` of `continuous` | `on_flush` | `manual`, and grant operations
  `workspace.read` (broad) / `workspace.write` (session-confined) / `workspace.push`
  (one allowed folder).

## Capabilities

### New Capabilities
- `workspaces`: session/run-scoped, object-store-backed workspaces — attach a
  folder mounted rw into the sandbox and synced to remote under the session prefix,
  a read-only shared root with grant-scoped push of one allowed folder, plus base
  code and lifecycle hooks the sandbox runs around the agent.

## Impact

- **Depends on `c0008-object-storage`**: workspaces sync to the `ObjectStore` port
  and use its presigned / hub-proxied path so store credentials stay server-side.
- **Depends on `c0009-sessions`**: workspaces are attached to a session and are
  isolated per session.
- **Depends on `c0006-sandbox-runtime`**: the sandbox driver provides Mount,
  Bootstrap, and RunHooks and preserves the stdin-not-argv invariant for hooks.
- New module homes: `client/workspacefs`, `server/storage` (`WorkspaceManager` +
  sync), and the `sandbox` runtime's Mount/Bootstrap/RunHooks.
- Behaviors are pinned by the e2e scenarios `WS-01` (attach mounts rw), `WS-02`
  (sandbox writes a real file), `WS-03` (writes sync to remote under the session
  prefix), `WS-04` (detach flushes then unmounts), `WS-05` (force-sync + observable
  status), `WS-06` (re-attach restores from remote / resume), `WS-07` (per-session
  isolation), `WS-08` (write outside the allowed prefix denied incl. traversal),
  `WS-09` (agent never holds store credentials); `SHARE-01` (read across the shared
  root), `SHARE-02` (shared root read-only), `SHARE-03` (push only the
  grant-allowed folder), `SHARE-04` (push outside denied), `SHARE-05` (published
  folder readable by other sessions), `SHARE-06` (grant scopes read-broad /
  write / push-narrow) in `tests/e2e/23_workspace_storage_test.go`; and `WSHOOK-01`
  / `WSHOOK-07` (base code via git / archive / store), `WSHOOK-02` (init/setup hooks
  run before the agent), `WSHOOK-03` (pre/post-run hooks bracket the agent),
  `WSHOOK-04` (failing init aborts the run + teardown), `WSHOOK-05` (hooks run
  within the grant scope), `WSHOOK-06` (post_sync hook after the remote push),
  `WSHOOK-08` (hook scripts over stdin, never argv) in
  `tests/e2e/24_workspace_hooks_test.go`.
