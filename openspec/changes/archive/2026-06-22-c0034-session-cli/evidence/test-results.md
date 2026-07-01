# Test Results — c0034-session-cli

## New session CLI tests (`libs/client/cli/cmd_session_test.go`)

```
=== RUN   TestSessionList_Table
--- PASS: TestSessionList_Table (0.00s)
=== RUN   TestSessionList_JSON
--- PASS: TestSessionList_JSON (0.00s)
=== RUN   TestSessionList_NoPAT
--- PASS: TestSessionList_NoPAT (0.00s)
=== RUN   TestSessionCreate
--- PASS: TestSessionCreate (0.00s)
=== RUN   TestSessionCreate_RequiresAgent
--- PASS: TestSessionCreate_RequiresAgent (0.00s)
=== RUN   TestSessionSend
--- PASS: TestSessionSend (0.00s)
=== RUN   TestSessionClose
--- PASS: TestSessionClose (0.00s)
=== RUN   TestSessionAttach_StreamsAndStopsOnDone
--- PASS: TestSessionAttach_StreamsAndStopsOnDone (0.00s)
=== RUN   TestSessionAttach_IdleTimeout
--- PASS: TestSessionAttach_IdleTimeout (0.15s)
PASS
ok  	mework/libs/client/cli	0.180s
```

## Coverage mapping to spec scenarios

| Scenario (specs/cli/spec.md) | Test |
|------------------------------|------|
| Create and inspect a session | TestSessionCreate, TestSessionList_Table |
| Send a turn and stream the reply | TestSessionSend, TestSessionAttach_StreamsAndStopsOnDone |
| Attach exits on idle | TestSessionAttach_IdleTimeout |
| Close a session | TestSessionClose |
| Machine-readable output | TestSessionList_JSON |
| Auth required (login guidance) | TestSessionList_NoPAT |

## Full suite

`make test` across all 6 modules completed with exit code 0 and no `FAIL`
lines; `libs/tests` integration suite ran in ~78s. `make vet` clean.
`openspec validate c0034-session-cli --strict` → valid.
