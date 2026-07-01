## Context

The redesign runs agents in isolated sandboxes (`c0006-sandbox-runtime`) for
sessions (`c0009-sessions`), backed by an S3-compatible object store
(`c0008-object-storage`). What is still missing is a durable, shareable **working
directory** for a run. Each agent currently works in an ephemeral host folder that
is destroyed with the sandbox: nothing persists the files, nothing lets a new
sandbox resume the same session's work, and there is no controlled channel for one
session to share outputs with another.

This change adds a `workspaces` capability over those three dependencies. The agent
sees a normal file view (`WorkspaceFS`); the server (`WorkspaceManager` in
`server/storage`) owns mounting, sync to the `ObjectStore`, the shared root, scoped
publish, base-code bootstrap, and lifecycle hooks; the sandbox driver provides the
mount, bootstrap, and hook execution primitives.

## Goals / Non-Goals

**Goals:**
- Attach a session-scoped folder, mounted **rw** into the sandbox, **synced to
  remote** under the session prefix so work is durable and resumable.
- A **read-only shared root** the agent can read across, with **push confined to
  one grant-allowed folder**.
- **Base code** (git / archive / store) and **lifecycle hooks** the sandbox runs
  around the agent, with hook scripts fed over **stdin, never argv**.
- The agent never holds raw store credentials.

**Non-Goals:**
- The object-store port and its drivers (owned by `c0008-object-storage`).
- Session identity / lifecycle (owned by `c0009-sessions`).
- Sandbox isolation mechanics and driver selection (owned by
  `c0006-sandbox-runtime`); this change only consumes Mount/Bootstrap/RunHooks.
- Grant issuance / authorization policy (owned by the runner/catalog changes); this
  change consumes the `workspace.read` / `workspace.write` / `workspace.push`
  operations.

## Decisions

- **`WorkspaceManager` (server/storage) is the only credential holder.** It exposes
  `Attach` (mount + bind to a remote prefix), `Get`, `Detach` (final flush then
  unmount), `Sync` (push/pull against the `ObjectStore`), `Status` (observable
  `pushed`/`pulled`/`failed` counts + last sync time), `MountSharedRoot` (read-only
  union of published folders), `Publish` (promote one allowed sub-path into the
  shared namespace), `Bootstrap` (materialize `Base` then run init hooks), and
  `RunHooks` (drive one lifecycle stage). Sync uses the object store's presigned /
  hub-proxied path so the agent never receives store access keys.
- **`WorkspaceFS` (client/workspacefs) is the agent-facing view.** `ReadFile` /
  `List` / `Stat` may **read across the shared root**; `WriteFile` / `Remove` are
  **confined to the grant's writable prefix**. All paths are normalized and
  **`..` traversal is blocked** — a write resolving outside the writable prefix
  (including via traversal) is denied.
- **`SyncMode` selects when remote pushes happen:** `continuous` mirrors writes as
  they happen, `on_flush` syncs on explicit flush / detach, `manual` only on an
  explicit `Sync()`. `Detach` always performs a final flush regardless of mode.
- **`BaseSpec` is a pluggable base source:** `git` (clone `Ref` @ `Rev`),
  `archive` (unpack an archive at `Ref`), or `store` (copy a template prefix from
  the `ObjectStore`). `Bootstrap` materializes the base into the workspace **before**
  any hook or the agent runs.
- **Hooks run at lifecycle `HookStage`s:** `init` (after base is materialized),
  `pre_run` (before the agent), `post_run` (after the agent), `pre_sync` (before the
  remote push), `post_sync` (after the remote push). Hook **scripts are delivered
  over stdin, never argv**, preserving the injection-safe invariant. A failing
  `init`/`pre_run` hook **aborts the run** (reported failed) and the sandbox is torn
  down. Hooks execute **within the workspace grant scope** — they may write the
  workspace but cannot exceed the grant (e.g. no network if the grant omits it).
- **Grant operations are distinct, least-privilege scopes:** `workspace.read`
  (broad, across the shared root), `workspace.write` (narrow, session-confined),
  and `workspace.push` (narrow, the one allowed publish destination). Read is broad
  while write and push are confined.
- **Per-session isolation.** Each session's workspace is bound to its own remote
  prefix; one session's writes are invisible to another session's workspace except
  through an explicitly published folder in the shared root.

## Risks / Trade-offs

- **Sync consistency vs. latency.** `continuous` minimizes loss but adds per-write
  overhead; `on_flush` / `manual` reduce overhead but widen the loss window — hence
  `Detach` always force-flushes and `Status` makes drift observable.
- **Traversal / scope enforcement is security-critical.** The deny path for writes
  outside the prefix (including `..`) must be airtight; it is enforced at the
  `WorkspaceFS` boundary and re-checked server-side at publish/sync.
- **Hook execution is attack surface.** Hook scripts come from spec/config and run
  in the sandbox; the stdin-not-argv invariant and grant-scope confinement bound the
  blast radius, but a misconfigured grant could still over-permit — keep hook grants
  least-privilege.
- **Resume semantics.** Re-attach pulls prior files from remote; large workspaces
  make rehydration slow, and concurrent attachers to one prefix could race — kept out
  of scope here by one-active-mount-per-session.
- **Base-source breadth.** Supporting git / archive / store keeps the base pluggable
  but each source is a distinct failure mode (auth, unpack, copy) that bootstrap must
  surface as a failed `Result`.
