# Quality gates — c0037-sandbox-cli

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | PASS (no output) |
| Vet | `go vet ./...` | PASS (exit 0) |
| Tests | `go test -p 1 ./...` | PASS (all `ok` / no-test packages; 0 failures) |
| OpenSpec | `openspec validate c0037-sandbox-cli --strict` | PASS — "Change 'c0037-sandbox-cli' is valid" |

Toolchain: go1.26.4 (satisfies go.mod's go 1.25.x requirement).
DB-backed tests skip without `TEST_DATABASE_URL` (not set in this run); that is
expected and does not affect the CLI-only change.

## Scope verified
- New `sandbox` command group registered under `groupRuntime` alongside
  `daemon`, `session`, `server`.
- `sandbox start -w <dir>` (default `.`): abs-path resolution + `mework.yml`
  validation (`catalog.LoadWorkspaceConfig`) + local runner resolution
  (`config.LoadIdentity`) before any network call; POSTs
  `{agent_name, version, runner, workspace=<abs>}` to `/api/v1/sessions`; prints
  the session id (`--json` for the full row); `--attach` streams via the session
  attach handler.
- `sandbox list/stop/send` delegate to the shipped `session` commands (no logic
  duplication); message content travels in the JSON body, never argv.
- Failure paths: missing `mework.yml` errors before the network; an unenrolled
  machine fails with guidance to enroll / start the daemon.
