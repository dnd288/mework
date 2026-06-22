## Why

A user wants to treat a **workspace** as the unit of work: a project directory holding a
`mework.yml` config and a `.claude/` settings dir. They want to **register** that
workspace into the server, **invoke the agent against it** from a remote session, and see
**results land back in the workspace** (inspect/update artifacts). They also want the
workspace to be portable — **pack / push / pull** to the server — and to run **fully on
the client machine** with no server at all.

Today the pieces are split and several seams are missing: the runner's
`DefinitionResolver` had only test fakes (now fixed — a catalog HTTP resolver landed); a
run/session was not bound to a workspace dir (now fixed — `RunSpec.Workspace` landed);
and there is no `mework.yml` config convention, no workspace pack/push/pull, and no
local-direct start path.

This change completes that picture, staying within the c0027 boundary (server = gateway +
registry; the daemon + sandbox run on the client): the **config is `mework.yml`**, a
workspace can be **packed/pushed/pulled** to the server, and it can be **started two
ways** — pulled from the server, or **fully locally** (`mework auth` + `mework daemon`
start the workspace as a local sandbox, no server).

## What Changes

- **`mework.yml` workspace config.** A per-workspace config file named `mework.yml`
  (engine + backend + image, plus workspace settings) is the registrable/portable unit.
  It decodes to the prebuilt-definition metadata. A **local file resolver** loads
  `mework.yml` from a workspace directory for the no-server path.
- **Workspace bound to a session/run.** *(landed)* `core.RunSpec.Workspace` + the runner
  thread a workspace dir into the run; the `local` engine runs the agent in it; container
  engines `Mount` it. Artifacts persist in the workspace and are read back via
  `client/workspacefs`.
- **Catalog HTTP resolver.** *(landed)* Resolve a registered config by reference from the
  server catalog (`GET /api/v1/agents/{name}?version=`).
- **Workspace pack / push / pull.** `pack` archives a workspace (`mework.yml` at root +
  files) into a bundle; `push` registers it to the server catalog; `pull` fetches and
  extracts it back into a local workspace directory. Exposed as a CLI surface.
- **Two start modes.**
  1. **Server:** pull the registered config (or bundle) from the server, then open a
     workspace-bound session.
  2. **Local-direct (fully on client):** `mework daemon` reads the local `mework.yml` and
     starts the workspace as a **local sandbox** with no server; `mework auth` supplies
     the local identity/grant.
- **End-to-end example (`examples/remote-claude`).** A workspace fixture
  (`mework.yml` + `.claude/settings.json` + a deterministic **stub backend**) that
  demonstrates **both** start modes, a **pack → push → pull** round-trip, and reading
  back / updating the agent's artifacts.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: adds (1) resolving a registered config by reference, (2)
  binding a workspace dir to a session/run, (3) the `mework.yml` workspace config,
  (4) workspace pack/push/pull to the server, and (5) two start modes — server-pulled and
  local-direct (fully on client).

## Impact

- **Sequenced after** `c0026-prebuilt-agent-sandbox` and `c0027-execution-locality`.
- **Landed already (this change):** `core.RunSpec.Workspace` (`libs/shared/core`), local
  engine workdir binding (`libs/sandbox/engine/local`), runner threading
  (`libs/client/runner`), and `libs/client/catalog` `HTTPDefinitionResolver`.
- **New code:**
  - `libs/client/catalog` — a **file resolver** that loads `mework.yml` from a dir.
  - workspace **pack/push/pull** (client; archive a workspace, register/fetch via the
    catalog bundle form) + a **CLI** surface (`mework workspace pack|push|pull`).
  - **local-direct start** path: daemon/runner starts a workspace from local `mework.yml`
    as a local sandbox using a local grant from `mework auth`.
  - Server catalog accepts `mework.yml` as the bundle manifest (minimal validation
    update; reuses the existing `bundle` form + storage).
- **Server:** reuses `GET /api/v1/agents/{name}` (resolve), `POST
  /api/v1/agents/{name}/versions` (publish, forms `definition` + `bundle`). The example's
  real-server harness **requires Postgres** (`TEST_DATABASE_URL`), gated to skip cleanly.
- No schema migration; stdin-not-argv, one-agent-per-sandbox, and the c0027 boundary
  (server never spawns a sandbox) are all preserved.
