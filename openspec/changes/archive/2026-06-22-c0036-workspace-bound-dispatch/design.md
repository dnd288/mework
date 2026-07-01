## Context

c0033's `processSessionDispatch` builds `SessionDeps` with a catalog `HTTPDefinitionResolver`
and no workspace. To make a server-routed session *be* a local workspace, the create request
must carry a workspace path that reaches the daemon, which then resolves from `mework.yml` in
that dir and binds the sandbox there. All the binding machinery (`RunSpec.Workspace`,
`SessionDeps.Workspace`, `FileDefinitionResolver`) already exists.

## Goals / Non-Goals

**Goals:**
- A `POST /sessions` with a workspace path opens the session bound to that local dir,
  resolving the definition from `<dir>/mework.yml`.
- Backward-compatible: no workspace → today's catalog path.

**Non-Goals:**
- The `sandbox` CLI (c0037) and `server start` (c0035).
- Shipping the workspace files to a remote daemon (the daemon is the same host; path is
  absolute and local). Cross-host workspace transfer stays the pack/push/pull story.

## Decisions

- **`Dispatch.Workspace` (absolute path) is the signal.** Non-empty → workspace-bound open.
  Keeps the wire minimal; no new message kind.
- **Second injected resolver factory.** The runner package can't import
  `libs/client/catalog` (cycle), hence the existing `SetSessionResolverFactory` seam. Add a
  parallel `SetSessionWorkspaceResolverFactory(func(path string) DefinitionResolver)` that the
  daemon wires to `func(p) DefinitionResolver { return &catalog.FileDefinitionResolver{WorkspaceDir: p} }`.
  In `processSessionDispatch`, choose the workspace factory when `d.Workspace != ""` and set
  `deps.Workspace = core.Workspace{Path: d.Workspace}`.
- **Definition name/version still flow on the dispatch** (from create), but the file resolver
  reads `mework.yml` as authoritative (its `ResolveDefinition` ignores the ref). The agent ref
  is used only for session metadata.
- **Server stays a relay.** It just forwards the workspace string; it never reads the
  workspace (the c0027 boundary holds — only the runner touches files).

## Risks / Trade-offs

- [Path must exist on the daemon host] → if `<dir>/mework.yml` is missing/unreadable on the
  daemon, `OpenSession` fails and the daemon reports the session failed (existing error path).
  c0037's CLI validates the path locally first to fail fast.
- [Relative vs absolute] → the caller (c0037) resolves to an absolute path before sending;
  the daemon does not guess a base dir. Document this.
- [Trust] → local engine has no isolation; binding a host dir gives the agent direct FS
  access (unchanged, trusted-only). Container engines isolate via `Mount`.

## Migration Plan

Additive. `Dispatch.Workspace` and `createSessionRequest.Workspace` are optional; the daemon
branch only activates on a non-empty workspace, so existing catalog-resolved sessions are
unaffected.
