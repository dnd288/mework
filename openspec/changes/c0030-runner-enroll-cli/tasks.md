## 1. Wire the enroll command (TDD)

- [x] 1.1 Write `libs/client/cli/cmd_runner_test.go` (fail first): table test with an
      `httptest.Server` stubbing `POST /api/v1/runners/enroll`. Cases: (a) 200 with
      `{runner_id, secret}` → identity file written + stdout contains the runner ID;
      (b) 401/4xx → command returns a non-nil error; (c) missing `--url` or `--token` →
      required-flag error. Sandbox the identity path so the real `~/.mework` is untouched.
- [x] 1.2 Replace the stub body of `runnerEnrollCmd.RunE` (`libs/client/cli/cmd_runner.go`):
      after parsing `--url`/`--token`, call `enroll.Enroll(cmd.Context(), url, token)` and
      print the returned `RunnerID`. Remove the `runner-%x` fabrication and the `bad-token`
      shim. Keep the manual flag parsing and required-flag checks.
- [x] 1.3 Remove now-dead helpers (e.g. `quickLen`) if no longer referenced.

## 2. Validation

- [x] 2.1 `make vet` and `make test ./libs/client/cli/...` green; new test fails before the
      wiring and passes after.
- [ ] 2.2 `make test` (full) green.
- [ ] 2.3 Manual smoke (optional): against a running server, `mework runner enroll --url
      <hub> --token <reg>` writes `~/.mework/identity.json` and prints the runner ID;
      `mework daemon start` then loads it.
