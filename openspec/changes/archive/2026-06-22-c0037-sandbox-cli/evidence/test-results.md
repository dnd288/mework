# Test results — c0037-sandbox-cli

## New tests (libs/client/cli/cmd_sandbox_test.go)

```
=== RUN   TestSandboxStart_PostsWorkspaceBoundSession
--- PASS: TestSandboxStart_PostsWorkspaceBoundSession (0.00s)
=== RUN   TestSandboxStart_JSON
--- PASS: TestSandboxStart_JSON (0.00s)
=== RUN   TestSandboxStart_MissingWorkspaceConfig
--- PASS: TestSandboxStart_MissingWorkspaceConfig (0.00s)
=== RUN   TestSandboxStart_NotEnrolled
--- PASS: TestSandboxStart_NotEnrolled (0.00s)
=== RUN   TestSandboxList
--- PASS: TestSandboxList (0.00s)
=== RUN   TestSandboxStop
--- PASS: TestSandboxStop (0.00s)
=== RUN   TestSandboxSend
--- PASS: TestSandboxSend (0.00s)
=== RUN   TestSandboxCommandSurface
--- PASS: TestSandboxCommandSurface (0.00s)
PASS
ok  	mework/libs/client/cli	0.069s
```

These map to the delta-spec scenarios:
- Start a workspace as a worker → TestSandboxStart_PostsWorkspaceBoundSession (+ _JSON)
- Sandbox start without a workspace config → TestSandboxStart_MissingWorkspaceConfig
- Sandbox start when not enrolled → TestSandboxStart_NotEnrolled
- Message a worker by id → TestSandboxSend
- Command surface → TestSandboxCommandSurface (+ TestSandboxList, TestSandboxStop)

## RED→GREEN

RED: `go test ./libs/client/cli/...` failed to compile —
`undefined: sandboxStartCmd / sandboxListCmd / sandboxStopCmd / sandboxSendCmd /
sandboxCmd` (test written first).

GREEN: after adding `cmd_sandbox.go` and registering the group in `help.go`,
`go test ./libs/client/cli/...` → `ok`.

## Full suite

`go test -p 1 ./...` → all packages `ok` or `[no test files]`; 0 failures.
(DB-backed tests skip without `TEST_DATABASE_URL`.)
