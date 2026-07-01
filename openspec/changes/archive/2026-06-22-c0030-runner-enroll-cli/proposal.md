## Why

To connect a local daemon to the server, an operator runs
`mework runner enroll --url <hub> --token <registration-token>`. The CLI baseline spec
already promises this enrolls the runner with a durable identity. **The command is a stub
today**: `runnerEnrollCmd.RunE` (`libs/client/cli/cmd_runner.go:30`) parses the flags but
then prints a fabricated `already enrolled. RunnerID: runner-<hex>` and **never makes an
HTTP call, never persists an identity**. It also short-circuits a literal `bad-token` as a
test shim.

The real enrollment library already exists and works: `enroll.Enroll(ctx, url, token)`
(`libs/client/enroll/enroll.go`) POSTs to `{url}/api/v1/runners/enroll`, receives
`{runner_id, secret}`, and persists it via `config.SaveIdentity`. The daemon's
`runForeground` (`libs/client/cli/daemon.go:164`) already loads that identity to start the
SSE engine. The only missing link is wiring the CLI command to the library.

This is the first step of the end-to-end "turn a workspace into a server-routed sandbox"
workflow (enroll → create session → chat). Without it, step 2 (auth + connect the daemon)
cannot be done through the CLI.

## What Changes

- **`mework runner enroll` performs the real handshake.** `runnerEnrollCmd.RunE` calls
  `enroll.Enroll(cmd.Context(), url, token)` and prints the returned `RunnerID`. The
  durable identity is persisted by the library (`config.SaveIdentity`) so a subsequent
  `mework daemon start` runs unattended.
- **Errors surface.** A hub rejection (non-2xx, e.g. an invalid/expired registration
  token) is returned as a CLI error, not swallowed. The fabricated `runner-<hex>` success
  branch and the `bad-token` literal shim are removed; failure behavior is exercised
  against a real `httptest` stub instead.
- **Required-flag validation is preserved.** `--url` and `--token` remain required; the
  existing manual flag parsing (which avoids cobra's persistent `pflag.Changed` state
  across sequential `Execute()` calls in tests) stays.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cli`: the **Command surface** requirement's runner-enrollment scenario is sharpened —
  `runner enroll` now performs a real HTTP enrollment handshake, persists a durable
  identity, and surfaces hub rejections as errors.

## Impact

- **Code:** `libs/client/cli/cmd_runner.go` (replace the stub body of
  `runnerEnrollCmd.RunE`; keep flag parsing). New test `libs/client/cli/cmd_runner_test.go`.
- **Reuses** the existing `enroll.Enroll` + `config.SaveIdentity` (no new persistence code)
  and the server's already-mounted `POST /api/v1/runners/enroll`
  (`libs/server/hub/router.go:142`).
- No server change, no schema migration. File perms for the identity file are owned by
  `config.SaveIdentity` (unchanged, `0600`).
- **First in sequence** for the interactive-session workflow; `c0031`–`c0034` build on an
  enrolled, daemon-connected runner.
