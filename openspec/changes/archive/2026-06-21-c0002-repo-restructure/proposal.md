## Why

The repo's layout grew organically and now mixes concerns in ways that block
large-team, parallel development and independent release. `internal/mello` is
imported by **both** the client (`cmd/mework`) and the server (`internal/server/*`);
client packages (`cli`, `daemon`, `agentrun`, `meworkclient`) and server packages
(`internal/server/*`) sit as flat siblings with no enforced boundary; infra
(`store`, `secret`, `token`) is interleaved with server domain logic. There is no
single home for the **client↔server contract** (the SSE/API DTOs) the redesign
introduces, and **no boundary that lets the sandbox runtime grow swappable engines**
(local, docker, cloud) without dragging heavy dependencies into every binary.

We want to **maintain the system as three independently-owned components — `client`,
`server`, and `sandbox` — that can become separate repositories later**, with one
small shared contract they agree on. The sandbox is a runtime of its own: e.g. its
docker engine **pulls and runs an agent image (claude code / codex preinstalled),
runs the workspace hooks (clone repo, setup), and mounts the synced workspace**.

The four redesign changes (`message-bus`, `agent-catalog`, `agent-runner`,
`sandbox-runtime`) touch client, server, sandbox, and shared-contract code at once.
Without clean module boundaries first they collide, can't be tested in isolation,
and can't be released or owned independently. **This change restructures the repo
first** so each component is developed, tested, released, and extended on its own —
and the redesign proceeds in parallel.

## What Changes

- Split the codebase into a **`go.work` workspace of four Go modules** — three
  components plus one shared contract — each with its own `go.mod`, independently
  buildable/testable/releasable, and ready to become its own repository later:
  - **`shared`** — the published contract: canonical domain types (`core`), the wire
    contracts (`transport`: SSE event schema + API DTOs + the runner↔sandbox
    protocol), the **pluggable interfaces** (`ports`: `SandboxDriver`/`Sandbox`,
    `ObjectStore`, `AgentBackend`, `Broker`, `ProviderAdapter`, `Notifier`), grant
    sign/verify primitives, config, errors/log, and a generic driver registry
    (`plugin`). The `mello` REST SDK lives here (used by both client and server).
  - **`server`** — the hub: registry, session, catalog, orchestrator, permission,
    scheduler/quota/audit/notify, webhook, writeback, auth/middleware; the `bus`,
    `storage`, and `provider` driver families; and server-only infra under
    `platform/{store,secret,token}`.
  - **`client`** — the CLI + runner (enroll → subscribe → pull → run → report),
    `subscribe` (SSE consumer), `workspacefs`, and OS specifics (`osproc`).
  - **`sandbox`** — the sandbox runtime + engines: `runtime` (one-agent-per-sandbox
    lifecycle, hooks, workspace mount, teardown) and `engine/{local,docker,
    cloudflare,custom}`, each engine isolating its own SDK.
- Enforce a **module-level dependency DAG**: `shared` is the leaf; `server → shared`;
  `sandbox → shared`; `client → shared + sandbox`. **`client ⟂ server`,
  `server ⟂ sandbox`, `server ⟂ client`** — the server never links a sandbox engine
  or the CLI; the sandbox never links server infra.
- Make every swappable backend a **plugin**: an interface (port) in `shared`, an
  adapter per backend in its own subpackage that isolates its dependency, registered
  via the registry, with the **binary selecting which drivers compile in** (blank
  import). A local-only client links no docker/S3/NATS SDK.
- The client **embeds** the sandbox runtime (one `mework` binary runs local/docker
  sandboxes out of the box); the sandbox module **also ships a standalone
  `mework-sandbox` daemon** for remote/isolated hosts and cloud engines.
- Drop `github.com/mark3labs/mcp-go` from `go.mod` (declared but imported nowhere).
- This is a **pure structural refactor**: no runtime behavior changes; the
  `mework` and `mework-server` binaries and their outputs stay identical.

## Capabilities

### New Capabilities
- `project-structure`: the multi-module workspace, module-boundary dependency rules,
  per-module build/test boundaries, and pluggable driver/engine architecture that
  make independent development, testing, release, repo-splitting, and extension
  possible.

## Impact

- Moves (no behavior change) across `cmd/*` and all of `internal/*` into the four
  module trees; every import path updates to its module path.
- New `shared/transport` becomes the single home for the SSE/API + runner↔sandbox
  contract the redesign depends on; new `shared/ports` is the home for every
  pluggable interface.
- **Sequenced FIRST**: `message-bus`, `agent-catalog`, `agent-runner`, and
  `sandbox-runtime` rebase onto these modules and reference the new paths (the
  sandbox engines land under the `sandbox` module).
- `go.work` + per-module `go.mod`; `Makefile` gains per-module build/test targets;
  CI runs each module's suite independently; goreleaser keeps cross-compiling the
  per-OS `mework` clients.
- `go.mod` loses `mcp-go`; heavy driver deps (docker SDK, S3 SDK, NATS) stay confined
  to their engine/driver subpackages and never link into components that don't use
  them.
- Risk is mechanical (module carve-out + import churn); mitigated by gofmt/goimports,
  `go.work`, and the unchanged test suite acting as a behavior guard.
