# Repository Split Guide

This document describes how each module in the mework workspace can be extracted
into its own Git repository, relying only on the published `mework/shared`
contract module.

## Module dependency DAG

All dependencies flow outward toward `shared`:

```
shared (leaf — no mework imports, no heavy third-party deps)
  ↑         ↑
server   sandbox
            ↑
          client
```

- `server` imports only `shared`.
- `sandbox` imports only `shared`.
- `client` imports `shared` and `sandbox`.
- `client` and `server` must never import each other.
- `server` must never import `sandbox`.
- `shared` must never import `client`, `server`, or `sandbox`.

## Future repository names

Each module or driver/engine subpackage maps to a separate repository:

| Module | Future repo | Published as |
|--------|-------------|--------------|
| `shared` | `mework-shared` | `mework/shared` |
| `server` | `mework-server` | `mework/server` |
| `client` | `mework-client` | `mework/client` |
| `sandbox` | `mework-sandbox` | `mework/sandbox` |
| `server/bus/postgres` | `mework-bus-postgres` | `mework/server/bus/postgres` |
| `server/bus/nats` | `mework-bus-nats` | `mework/server/bus/nats` |
| `server/storage/s3` | `mework-storage-s3` | `mework/server/storage/s3` |
| `server/storage/minio` | `mework-storage-minio` | `mework/server/storage/minio` |
| `sandbox/engine/docker` | `mework-sandbox-docker` | `mework/sandbox/engine/docker` |
| `sandbox/engine/cloudflare` | `mework-sandbox-cloudflare` | `mework/sandbox/engine/cloudflare` |
| `server/provider/github` | `mework-provider-github` | `mework/server/provider/github` |
| `server/provider/jira` | `mework-provider-jira` | `mework/server/provider/jira` |

## Extraction procedure

### 1. Publish `mework/shared` first

Because every other module depends on `shared`, it must be published first:

```bash
# Extract the shared module into its own repository:
git subtree split --prefix=shared -b shared-repo
git push git@github.com:your-org/mework-shared.git shared-repo:main
```

Tag a release so other modules can depend on a specific version:

```bash
git tag v0.1.0
git push origin v0.1.0
```

### 2. Extract each component repo

For each module (`server`, `client`, `sandbox`):

```bash
# Extract the module's directory into a new branch:
git subtree split --prefix=<module> -b <module>-repo

# Push to its own repository:
git push git@github.com:your-org/mework-<module>.git <module>-repo:main
```

### 3. Update `go.mod` replace directives

In each extracted repository, replace the local `replace` directive with a
published module reference:

**Before (local development with `go.work`):**

```
replace mework/shared => ../shared
```

**After (standalone repository):**

```
require mework/shared v0.1.0
```

### 4. Keep `go.work` for local development

The `go.work` file in the monorepo roots ties all modules together without
needing to publish intermediate changes:

```
go 1.25.7

use (
    ./shared
    ./server
    ./client
    ./sandbox
)
```

When a module moves to its own repo, replace its `use` line with a
`replace` directive pointing to the local checkout, or remove it and
let Go resolve from the published module proxy.

## Engine/driver subpackage extraction

Each engine or driver subpackage follows the same pattern. For example, the
Docker sandbox engine lives at `sandbox/engine/docker/`. It depends only on
`mework/shared` (the `SandboxDriver`/`Sandbox` ports), so it can become its
own repository:

```bash
git subtree split --prefix=sandbox/engine/docker -b docker-engine-repo
git push git@github.com:your-org/mework-sandbox-docker.git docker-engine-repo:main
```

The extracted repo's `go.mod` gains:

```
require mework/shared v0.1.0
```

And consumers import only the port from `shared/ports`:

```go
import "mework/shared/ports"
```

The concrete driver is wired at the binary level via blank import:

```go
import _ "mework-sandbox-docker"
```

## Rules of thumb

- **No circular dependencies.** A module must never import a sibling that
  depends on it.
- **No cross-engine imports.** An engine may import its own SDK but must never
  import another engine.
- **No heavy deps in `shared`.** The `shared` module may only use the standard
  library and net/http (for the Mello SDK).
- **Binaries wire, they don't contain logic.** All domain logic lives in
  non-`cmd` packages; binaries only import and wire.
- **Plug via interfaces, not concrete types.** Consumers import ports from
  `shared/ports`; drivers register themselves at init time.
