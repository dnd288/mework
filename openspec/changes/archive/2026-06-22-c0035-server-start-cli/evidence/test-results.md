# Test results — c0035-server-start-cli

## `go test ./cli/ -run TestServerStart -v`

```
=== RUN   TestServerStart
=== RUN   TestServerStart/invokes_wired_starter_with_listen_override
=== RUN   TestServerStart/empty_listen_passed_when_flag_omitted_(no_override)
=== RUN   TestServerStart/no_starter_wired_returns_clear_error
--- PASS: TestServerStart (0.00s)
    --- PASS: TestServerStart/invokes_wired_starter_with_listen_override (0.00s)
    --- PASS: TestServerStart/empty_listen_passed_when_flag_omitted_(no_override) (0.00s)
    --- PASS: TestServerStart/no_starter_wired_returns_clear_error (0.00s)
PASS
ok  	mework/libs/client/cli	0.003s
```
