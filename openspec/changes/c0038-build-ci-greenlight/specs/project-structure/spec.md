## MODIFIED Requirements

### Requirement: Independent module build and test

Each module SHALL build and test **without** the other components' packages, and the
build SHALL expose per-module targets so each module's suite runs independently. The
top-level `make build` and the per-module `build-*`/`test-*` targets SHALL resolve to
the repository's actual workspace paths — binaries under `apps/` and modules under
`libs/<name>` — and SHALL succeed when run from a clean checkout.

#### Scenario: Server builds without client or sandbox

- **WHEN** CI runs the server build/test target
- **THEN** it compiles and tests `server` (and `shared`) without requiring any `client` or `sandbox` package

#### Scenario: Client builds without server

- **WHEN** CI runs the client build/test target
- **THEN** it compiles and tests `client` (with `shared` and `sandbox`) without requiring any `server` package

#### Scenario: Top-level build targets resolve to real paths

- **WHEN** a developer runs `make build`
- **THEN** both binaries are produced from `./apps/mework` and `./apps/mework-server` and the command exits zero, with no "directory not found" error

#### Scenario: Per-module targets resolve to libs paths

- **WHEN** any `make build-<module>` or `make test-<module>` target runs (for `shared`, `server`, `client`, `sandbox`)
- **THEN** it operates in the corresponding `libs/<module>` directory and exits zero, with no "can't cd" error

## ADDED Requirements

### Requirement: Consistent Go toolchain across module, workspace, and CI

The declared Go toolchain SHALL be consistent across `go.mod`, `go.work`, and the CI
workflow so that the workspace builds and tests with the pinned toolchain without
relying on implicit toolchain auto-download.

#### Scenario: Module and workspace agree on the toolchain

- **WHEN** the `go` directive in `go.mod` is compared to the `go` directive in `go.work`
- **THEN** neither requires a newer Go than the other declares, so a toolchain that satisfies `go.mod` can build the full workspace

#### Scenario: CI uses a satisfying toolchain

- **WHEN** the CI workflow resolves its Go version
- **THEN** the resolved version satisfies both `go.mod` and `go.work`, and `go build ./...` across the workspace succeeds in CI
