# Gates — c0030-runner-enroll-cli

Toolchain: go1.26.4 (>= 1.25 required by go.mod).

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | PASS |
| Vet | `make vet` | PASS |
| Test (full) | `make test` (`go test -p 1 ./...`) | PASS — all packages ok |
| Unit (changed pkg) | `go test ./libs/client/cli/ -run TestRunnerEnroll` | PASS (4/4 subtests) |
| Spec validate | `openspec validate c0030-runner-enroll-cli --strict` | PASS — "Change is valid" |

## TDD evidence

- RED: `TestRunnerEnroll/success_persists_identity_and_prints_runner_id` failed
  before wiring — the stub fabricated `runner-<hex>` and never persisted an
  identity (`stdout ... does not contain "runner-abc123"`; `identity saved = false`).
- GREEN: after wiring `runnerEnrollCmd.RunE` to `enroll.Enroll(cmd.Context(), url, token)`
  and removing the `runner-%x` fabrication, the `bad-token` shim, and the dead
  `quickLen` helper, all 4 subtests pass.

## Coverage (changed packages)

- `libs/client/enroll` Enroll: 76.2%
- `libs/client/cli`: 16.9% (package-wide; the enroll command path is now exercised)

See `coverage.txt`.
