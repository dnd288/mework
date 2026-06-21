## 1. Workspace types & manager interface

- [ ] 1.1 Define `WorkspaceSpec` (mount path, remote prefix, mode, sync mode, shared roots, base, hooks), `Workspace`, `WorkspaceID`, `WorkspaceMode`, `SyncMode`, `SyncResult`, `BaseSpec`/`BaseKind`, `Hook`/`HookStage` in `client/workspacefs`
- [ ] 1.2 Define the `WorkspaceManager` interface (Attach/Get/Detach/Sync/Status/MountSharedRoot/Publish/Bootstrap/RunHooks) in `server/storage`
- [ ] 1.3 Define the `WorkspaceFS` interface (ReadFile/WriteFile/List/Remove/Stat) in `client/workspacefs`
- [ ] 1.4 Define the `workspace.read` / `workspace.write` / `workspace.push` grant operations

## 2. Attach, mount & per-session isolation (server/storage)

- [ ] 2.1 Implement `Attach`: bind a session-scoped folder to a remote prefix and mount it rw at the spec's mount path
- [ ] 2.2 Implement `Detach`: final flush then unmount
- [ ] 2.3 Enforce per-session isolation: bind each workspace to its own remote prefix; one session's writes are invisible to another's workspace

## 3. Sync to the object store (server/storage)

- [ ] 3.1 Implement `Sync` against the `ObjectStore` (push local writes under the session prefix; pull on re-attach)
- [ ] 3.2 Implement `SyncMode` behaviors: `continuous`, `on_flush`, `manual`
- [ ] 3.3 Implement `Status` reporting observable pushed/pulled/failed counts and last sync time
- [ ] 3.4 Route all sync through the object store's presigned / hub-proxied path so the agent never holds raw store credentials

## 4. Agent-facing file view (client/workspacefs)

- [ ] 4.1 Implement `ReadFile`/`List`/`Stat` with read across the shared root
- [ ] 4.2 Implement `WriteFile`/`Remove` confined to the grant's writable prefix
- [ ] 4.3 Normalize paths and block `..` traversal; deny writes resolving outside the writable prefix

## 5. Shared root & scoped publish (server/storage)

- [ ] 5.1 Implement `MountSharedRoot`: read-only union of published folders the agent may read across
- [ ] 5.2 Enforce the shared root is read-only (writes into it are denied)
- [ ] 5.3 Implement `Publish`: promote only the grant-allowed folder into the shared namespace; deny pushes outside the allowed destination
- [ ] 5.4 Make a published folder readable by other sessions through the shared root

## 6. Base code & lifecycle hooks (sandbox runtime)

- [ ] 6.1 Implement `Bootstrap`: materialize `BaseSpec` (`git` | `archive` | `store`) into the workspace before any hook or the agent
- [ ] 6.2 Run `init` hooks during bootstrap; a failing `init`/`pre_run` hook aborts the run (reported failed) and tears down the sandbox
- [ ] 6.3 Implement `RunHooks` driving `pre_run`/`post_run` to bracket the agent and `pre_sync`/`post_sync` around the remote push
- [ ] 6.4 Run hooks within the workspace grant scope (may write the workspace, cannot exceed the grant)
- [ ] 6.5 Feed hook scripts over stdin, never argv (preserve the injection-safe invariant)

## 7. Validate

- [ ] 7.1 openspec validate c0010-session-workspaces --type change --strict
- [ ] 7.2 e2e pointer: flip `tests/e2e/23_workspace_storage_test.go` from Skip to Green for WS-01..09 (folder→rw mount, sandbox writes a real file, sync-to-remote, detach-flush, force-sync+status, re-attach restores from remote, per-session isolation, write-outside-prefix denied, agent never holds store creds via presigned URLs) and SHARE-01..06 (shared root read-all, shared root read-only, push only the grant-allowed folder, push-outside denied, published folder readable by other sessions, grant scopes read(broad)/write/push(narrow)); flip `tests/e2e/24_workspace_hooks_test.go` from Skip to Green for WSHOOK-01..08 (base code + lifecycle hooks: init clones git, setup installs deps, pre_run/post_run bracket the agent, failing init aborts the run, hooks within grant scope, post_sync after remote push, base from archive/store template, hook scripts over stdin not argv).
