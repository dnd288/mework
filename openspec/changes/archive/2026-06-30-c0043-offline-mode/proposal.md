## Why

Running a mework agent currently requires a full server stack (Postgres, hub, provider). Developers who want to use mework agents locally — on their own workspace, triggered from their own terminal — have no way to do it without the server infrastructure. This change adds an **offline mode** that lets anyone run `mework start --workspace . --offline` and trigger agent execution directly from the CLI, with zero external dependencies.

## What Changes

- A new `mework start --workspace <dir> --offline` command that boots a self-contained local agent
- A new `mework run <instruction>` command that sends a one-shot task to the offline-mode agent
- The offline agent resolves its definition from the workspace's `mework.yml` (no catalog server)
- The agent executes via the existing local sandbox engine (backend=claude, codex, etc.)
- Results are streamed to stdout and optionally written as workspace artifacts
- **No Postgres, no hub, no Mello, no provider registration required**

## Capabilities

### New Capabilities

- `offline-agent`: A self-contained local agent lifecycle — start, run one-shot tasks, stop — with definition resolved from the workspace file system rather than a server catalog. Supports `--workspace` and `--offline` flags. No database, no webhook, no provider.

### Modified Capabilities

*(None — this is purely additive, orthogonal to the existing server-centric flow.)*

## Impact

- **New CLI commands**: `mework start` (parent), `mework run` (one-shot task execution)
- **Existing code reused**: `libs/runner/workspace_start.go` (workspace-bound session), `libs/sandbox/engine/local/` (local sandbox runner), `libs/shared/config/` (identity and config)
- **No server changes**: The hub, webhook, write-back, auth, provider packages are untouched
- **No database**: Offline mode uses no Postgres — state is in-memory and workspace files
- **Entry point**: `apps/mework/` CLI binary (not `mework-server`)
