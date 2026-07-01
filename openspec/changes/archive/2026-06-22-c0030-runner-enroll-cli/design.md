## Context

The `runner enroll` CLI command is a stub while the enrollment library
(`enroll.Enroll`), the identity persistence (`config.SaveIdentity` / `config.LoadIdentity`),
and the server endpoint (`POST /api/v1/runners/enroll`) are all real and already used by
`mework daemon start`. This change wires the command to the library — the smallest unit
that makes "connect the daemon" work end-to-end.

## Goals / Non-Goals

**Goals:**
- `mework runner enroll --url <hub> --token <reg>` exchanges the token for a durable
  identity and persists it, then prints the runner ID.
- Hub rejections surface as CLI errors.

**Non-Goals:**
- Re-enrollment / rotation flows, multi-runner identity selection, or changing the
  on-disk identity format (owned by `libs/shared/config`).
- Any server-side change to the enroll endpoint.

## Decisions

- **Call the existing library, don't reimplement.** `runnerEnrollCmd.RunE` calls
  `enroll.Enroll(cmd.Context(), url, token)`; persistence is the library's responsibility
  via `config.SaveIdentity`. Print `RunnerID` from the returned identity.
- **Keep manual flag parsing.** The command sets `DisableFlagParsing: true` and parses
  `--url`/`--token` by hand specifically because cobra's `pflag.Changed` persists across
  sequential `Execute()` calls and breaks required-flag detection in table tests. Preserve
  this; only the post-parse body changes.
- **Remove the test shims.** Delete the fabricated `runner-%x` success line and the
  `bad-token` literal branch. Failure is now tested by pointing `--url` at an `httptest`
  server that returns a non-2xx — exercising the real error path in `enroll.Enroll`.
- **Sandbox the identity write in tests.** The test isolates the identity file location
  (via the env/home indirection `config` already honors) so it never touches the
  developer's real `~/.mework`.

## Risks / Trade-offs

- [Tests previously relied on the `bad-token` shim] → replace with an `httptest` 4xx stub;
  the assertion becomes "error returned", which is stricter and real.
- [Context plumbing] → use `cmd.Context()` so the HTTP call is cancellable, matching other
  network commands.

## Migration Plan

Additive behavior change to one command. No config format change; an identity written by
the real handshake is the same shape `mework daemon start` already loads.
