## Context

Module `mework` (Go 1.25.7), two binaries, everything else flat under `internal/`.
Verified coupling: `internal/mello` is imported by both `cmd/mework` (client) and
`internal/server/*` (server) → genuinely shared; `internal/meworkclient` is
client-only; `internal/store`, `internal/server/secret`, `internal/server/token` are
server infra. Importantly, **there is no third-party dependency shared between client
and server today** (pgx/goose/chi are server-only; cobra client-only), and the heavy
driver deps the redesign needs (docker SDK, S3 SDK, NATS) are **not imported yet** —
so we can carve clean modules without untangling dependencies. `github.com/mark3labs/
mcp-go` is in `go.mod` but imported nowhere.

## Goals / Non-Goals

**Goals:**
- Three independently-owned, separately-releasable components — `client`, `server`,
  `sandbox` — over one small shared contract, each ready to become its own repo.
- Build and test each module without the others; keep heavy driver deps out of
  components that don't use them.
- One home for the wire contract and one home for every pluggable interface.
- Make backends (sandbox engines, storage, bus, providers, agents) swappable plugins.
- Keep it a pure refactor — zero behavior change — so it lands fast and de-risks the
  redesign that follows.

**Non-Goals:**
- Splitting into separate Git repositories now (documented as the next step; the
  module boundaries make it mechanical).
- Any runtime/behavior change, new feature, or dependency upgrade (besides dropping
  the unused `mcp-go`).
- Implementing the docker/cloud sandbox engines or the storage/bus drivers — those
  land in the downstream changes against the ports defined here.
- Rewriting the baseline specs (they update via sync as features land).

## Decisions

- **Four Go modules in a `go.work` workspace** — three components (`client`,
  `server`, `sandbox`) plus one leaf contract module (`shared`) — with a strict
  module-level dependency DAG: `shared` (leaf) ← `server`; `shared` ← `sandbox`;
  `shared` + `sandbox` ← `client`. **`client ⟂ server`, `server ⟂ sandbox`,
  `server ⟂ client`.**
- **`shared` is the published contract**, deliberately free of heavy third-party
  deps: `core` (domain types), `transport` (SSE/API DTOs + the runner↔sandbox
  protocol), `ports` (the pluggable interfaces), `grant` (sign/verify primitives both
  sides use), `config`, `errors`, `log`, and `plugin` (a generic `Register/Open`
  driver registry). The `mello` REST SDK lives in `shared` (used by client board/
  ticket commands and by the server's write-back adapter), avoiding a client↔server
  edge or a duplicated SDK.
- **`sandbox` is its own runtime module.** It owns `SandboxManager` lifecycle and an
  engine per backend under `engine/{local,docker,cloudflare,custom}`, each isolating
  its SDK. Engines implement the `SandboxDriver`/`Sandbox` port from `shared/ports`
  so the runner never branches on engine. The **docker engine** pulls and runs an
  agent image (claude code / codex preinstalled), clones the repo + runs `init` hooks
  via `Bootstrap`, attaches the workspace via `Mount`, runs the agent over stdin via
  `Exec`, and tears down via `Destroy` — the docker SDK is imported only there.
- **Pluggable everything via ports + a registry.** Each swappable backend (sandbox
  engine, `ObjectStore`, `Broker`, `ProviderAdapter`, `AgentBackend`, `Notifier`) is
  an interface in `shared/ports`; adapters self-register; the **binary** selects which
  drivers compile in via blank imports, so unused drivers and their deps never link.
- **Engine selection is capability/policy-driven.** A `SandboxDriver` advertises
  `SandboxCaps` (isolation, network, persistence, remote?, limits); the orchestrator +
  dispatch grant choose the engine per run (untrusted → docker/cloud; trusted/fast →
  local). Local/docker keep source + execution on the dev machine; cloud engines move
  execution off-host — a policy-gated, grant-surfaced choice, never a silent default.
- **Client embeds sandbox; sandbox also runs standalone.** One `mework` binary wires
  in the local/docker engines for out-of-the-box use; `cmd/mework-sandbox` runs the
  same runtime as a daemon for remote/isolated hosts and the out-of-process engine
  path.
- **Enforce the rule in CI** with an import-guard (e.g. `depguard`/`go-arch-lint`):
  no `client↔server`; engines may import their SDK but nothing imports another engine;
  `shared` imports nothing internal.
- **Mechanical migration** via `git mv` + per-module `go.mod` + `go.work` + module-wide
  import rewrite (`goimports`/`gofmt -r`), validated by the unchanged test suite.
- **Drop `mcp-go`** from `go.mod` (imported nowhere).

### Target layout

```
mework/                      # repo today; go.work ties the modules; each dir → own repo later
  go.work                    # use ./shared ./server ./client ./sandbox

  shared/                    # module mework/shared — contracts only, no heavy deps
    go.mod
    core/                    # domain types: Agent, Run, Session, Grant, Topic, Message,
                             #   RunSpec, Result, Workspace, ObjectRef/Info, Hook, SandboxCaps …
    transport/               # SSE event schema + API DTOs (client↔server) + runner↔sandbox protocol
    ports/                   # pluggable interfaces: SandboxDriver/Sandbox, ObjectStore,
                             #   AgentBackend, Broker, ProviderAdapter, Notifier
    grant/                   # grant sign/verify primitives (pure crypto; both sides)
    providers/mello/         # Mello REST SDK (was internal/mello) — used by client + server
    config/  errors/  log/  plugin/      # plugin/ = generic driver registry (Register/Open)

  server/                    # module mework/server — the hub
    go.mod                   # requires mework/shared
    cmd/mework-server/       # binary: wiring; blank-imports the server drivers it ships
    hub/ registry/ session/ catalog/ orchestrator/ permission/
    scheduler/ quota/ audit/ notify/ webhook/ writeback/ auth/ middleware/
    bus/  bus/{postgres,memory,nats}/         # Broker port + drivers
    storage/  storage/{s3,minio,r2,fs}/       # ObjectStore/Workspace + drivers
    provider/  provider/{mello,github,jira}/  # ProviderAdapter + adapters
    platform/{store,secret,token}/            # server-only infra (pgx/goose/AES/HMAC)

  client/                    # module mework/client — CLI + runner
    go.mod                   # requires mework/shared, mework/sandbox
    cmd/mework/              # binary: wiring; blank-imports the sandbox engines it ships
    cli/ runner/ subscribe/ workspacefs/ osproc/   # osproc: unix.go/windows.go (macOS rides unix)

  sandbox/                   # module mework/sandbox — the sandbox runtime + engines
    go.mod                   # requires mework/shared (+ per-engine SDKs, isolated)
    runtime/                 # SandboxManager: one-agent-per-sandbox lifecycle, hooks,
                             #   workspace mount, crash teardown
    agent/                   # AgentBackend port consumer + backends ─ claude/ codex/ opencode/
                             #   (local engine detects host CLI; docker engine uses the image's)
    engine/local/            # host subprocess (baseline)
    engine/docker/           # pull agent image (claude/codex preinstalled) → run → hooks →
                             #   mount workspace            [docker SDK only here]
    engine/cloudflare/       # remote cloud sandbox          [CF SDK only here]
    engine/custom/           # example out-of-tree engine + the contract doc
    cmd/mework-sandbox/      # optional standalone sandbox daemon (remote/separate host)
```

### Migration mapping (current → target)

| Current | Target |
|---|---|
| `internal/mello` | `shared/providers/mello` (SDK; used by client + server) |
| `internal/cli` (config primitives) | `shared/config` |
| `cmd/mework/cmd_*.go` | `client/cli` (binary keeps only wiring) |
| `internal/daemon` | `client/runner` |
| `internal/meworkclient` | `client/subscribe` |
| `internal/agentrun` (exec) | `sandbox/engine/local` |
| `internal/agentrun` (detect) | `sandbox/agent` (`AgentBackend` detection/registry) |
| `internal/server/*` (domain) | `server/*` (regrouped; root → `hub`; `profile` → `catalog`) |
| `internal/server/provider` (+ mello) | `server/provider` (+ `provider/mello`) |
| `internal/store` | `server/platform/store` |
| `internal/server/secret` | `server/platform/secret` |
| `internal/server/token` | `server/platform/token` |
| `cmd/mework/cmd_daemon_{unix,windows}.go` | `client/osproc` |
| `tests/e2e/api_test.go` (target interface types) | `shared/ports` (lifted source of truth) |
| `tests/e2e/harness_test.go` (imports `internal/store`, `internal/server/registry`) | imports update to `server/platform/store` and `server/registry` |
| `tests/e2e/*_test.go` (spec comments) | update module paths to match new layout |
| (new) | `shared/{ports,transport,grant,plugin}`, `sandbox/runtime`, `sandbox/agent`, `sandbox/engine/{docker,cloudflare,custom}`, `server/{bus,storage}` |

### Future repository split

Each module already depends only outward, so splitting to repos is a move, not a
rewrite. Likely repos: `mework-shared` (the contract, published/versioned first),
`mework-server`, `mework-client`, `mework-sandbox` — plus per-engine/driver repos
(`mework-sandbox-docker`, `mework-storage-s3`, `mework-provider-github`, …) that
depend only on `mework-shared`. `go.work` ties them locally during development.

## Risks / Trade-offs

- **Multi-module friction.** `go.work` + per-module `go.mod` adds coordination over a
  single module; accepted — it's the explicit goal (independent components + repos),
  and `go.work` keeps the local dev loop single-checkout.
- **Large carve-out / import churn.** Land in one focused PR with a freeze window;
  mechanical, with the unchanged test suite as the net.
- **Boundary erosion.** Without the CI import-guard the module rules rot; the guard is
  part of the change, not optional.
- **`shared` must stay dependency-free.** A heavy import in `shared` would leak into
  all components — the import-guard forbids third-party deps in `shared` (except the
  `mello` SDK, which is net/http only).
- **Driver registry discipline.** Wiring is by blank import in binaries; a missing
  blank import silently omits a driver — covered by per-module build/test targets and
  startup driver-presence checks.
- **`cli` vs `config` split.** `internal/cli` holds both CLI wiring and config
  primitives; config → `shared/config`, CLI → `client/cli`.
