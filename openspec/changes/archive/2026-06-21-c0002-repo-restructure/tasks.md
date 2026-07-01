## 1. Establish the shared contract module

- [x] 1.1 Create the `shared` module (`shared/go.mod`, module path `mework/shared`) with `core/`, `transport/`, `ports/`, `grant/`, `config/`, `errors/`, `log/`, `plugin/`
- [x] 1.2 Move the Mello SDK to `shared/providers/mello` (was `internal/mello`) and config primitives out of `internal/cli` into `shared/config`
- [x] 1.3 Add `shared/transport` as the single home for the SSE event schema + API DTOs + the runner↔sandbox protocol (stub now; the redesign fills it in)
- [x] 1.4 Add `shared/ports` with the pluggable interfaces (`SandboxDriver`/`Sandbox`, `ObjectStore`, `AgentBackend`, `Broker`, `ProviderAdapter`, `Notifier`) and `shared/plugin` with a generic `Register/Open` driver registry (stubs; downstream changes implement against them)
- [x] 1.5 Confirm `shared` imports nothing internal and pulls no heavy third-party dep (Mello SDK is net/http only)

## 2. Carve out the sandbox module

- [x] 2.1 Create the `sandbox` module (`sandbox/go.mod`, requires `mework/shared`) with `runtime/` (SandboxManager lifecycle: hooks, workspace mount, teardown)
- [x] 2.2 Move `internal/agentrun` execution → `sandbox/engine/local` (baseline engine) and backend detection → `sandbox/agent` (`AgentBackend` registry: claude/codex/opencode)
- [x] 2.3 Add empty `sandbox/engine/{docker,cloudflare,custom}` subpackages documenting the `SandboxDriver`/`Sandbox`/`SandboxCaps` contract (drivers land in `sandbox-runtime`); the docker engine's SDK will live only under `engine/docker`
- [x] 2.4 Add `sandbox/cmd/mework-sandbox` as the optional standalone sandbox daemon (wiring stub)
- [x] 2.5 Verify the `sandbox` module imports only `mework/shared`

## 3. Carve out the server module

- [x] 3.1 Create the `server` module (`server/go.mod`, requires `mework/shared`) with `cmd/mework-server` (wiring) and promote the old `internal/server` root into `server/hub`
- [x] 3.2 Move domain packages under `server/*` (registry, session, profile→`catalog`, webhook, jobs→orchestrator/queue, provider, auth, middleware) and server infra `internal/store|server/secret|server/token` → `server/platform/{store,secret,token}`
- [x] 3.3 Add driver families `server/bus` (+ `bus/{postgres,memory,nats}`) and `server/storage` (+ `storage/{s3,minio,r2,fs}`) and `server/provider` (+ `provider/{mello,github,jira}`) — ports consumed from `shared`, each driver isolating its SDK (stubs beyond the existing mello adapter)
- [x] 3.4 Verify the `server` module imports neither `mework/client` nor `mework/sandbox`

## 4. Carve out the client module

- [x] 4.1 Create the `client` module (`client/go.mod`, requires `mework/shared` and `mework/sandbox`) with `cmd/mework` (wiring)
- [x] 4.2 Move `cmd/mework/cmd_*.go` → `client/cli`, `internal/daemon` → `client/runner`, `internal/meworkclient` → `client/subscribe`, and the OS-detach files → `client/osproc` (unix/windows build tags; macOS rides unix); add `client/workspacefs` (WorkspaceFS stub)
- [x] 4.3 Wire the local/docker sandbox engines into `cmd/mework` via blank import; verify the client links no `mework/server` package

## 5. Workspace & wiring

- [x] 5.1 Add `go.work` (`use ./shared ./server ./client ./sandbox`) and per-module `go.mod`; module-wide import rewrite (`goimports`/`gofmt -r`)
- [x] 5.2 Each binary selects its drivers by blank import (`cmd/mework` → sandbox engines; `cmd/mework-server` → bus/storage/provider drivers); add a startup driver-presence check

## 6. Enforce boundaries

- [x] 6.1 Add an import-guard lint (e.g. `depguard`/`go-arch-lint`) encoding the module DAG: `shared` leaf; `server→shared`; `sandbox→shared`; `client→shared+sandbox`; `client⟂server`, `server⟂sandbox`, `server⟂client`
- [x] 6.2 Add a rule that an engine/driver subpackage may import its SDK but **no package imports another engine/driver**, and `shared` imports no heavy third-party dep
- [x] 6.3 Wire the guard into `make lint` and CI so boundary violations fail the build

## 7. Per-module build/test

- [x] 7.1 Add `Makefile` targets `build-{shared,server,client,sandbox}` and `test-{shared,server,client,sandbox}`, each operating on one module
- [x] 7.2 Confirm a local-only `mework` client builds without linking the docker/S3/NATS SDKs, and the server builds without the client or sandbox

## 8. Document the repo split

- [x] 8.1 Document how each module (`shared`, `server`, `client`, `sandbox`) and each engine/driver becomes its own repository depending only on `mework/shared`, with `go.work` tying them locally during development

## 9. Validate (behavior-preserving)

- [x] 9.1 `gofmt`/`goimports` clean; `go.work` builds both `mework` and `mework-server` binaries unchanged
- [x] 9.2 Full existing test suite passes with only import-path edits (no assertion changes); drop the unused `mark3labs/mcp-go` dependency
- [x] 9.3 `openspec validate --change repo-restructure --strict`
