# Test results — c0030-runner-enroll-cli

## go test ./libs/client/cli/ -run TestRunnerEnroll -v

```
=== RUN   TestRunnerEnroll
=== RUN   TestRunnerEnroll/success_persists_identity_and_prints_runner_id
=== RUN   TestRunnerEnroll/hub_rejection_returns_error_and_saves_nothing
=== RUN   TestRunnerEnroll/missing_url_fails_before_network_call
=== RUN   TestRunnerEnroll/missing_token_fails_before_network_call
--- PASS: TestRunnerEnroll (0.00s)
    --- PASS: TestRunnerEnroll/success_persists_identity_and_prints_runner_id (0.00s)
    --- PASS: TestRunnerEnroll/hub_rejection_returns_error_and_saves_nothing (0.00s)
    --- PASS: TestRunnerEnroll/missing_url_fails_before_network_call (0.00s)
    --- PASS: TestRunnerEnroll/missing_token_fails_before_network_call (0.00s)
PASS
ok  	mework/libs/client/cli	(cached)
```

## Full suite tail (make test)

```
--- libs/client ---
ok  	mework/libs/client	98.026s
ok  	mework/libs/client/catalog	0.102s
ok  	mework/libs/client/cli	0.141s
ok  	mework/libs/client/cmd/mework	0.029s
ok  	mework/libs/client/enroll	0.042s
?   	mework/libs/client/osproc	[no test files]
ok  	mework/libs/client/runner	4.679s
ok  	mework/libs/client/subscribe	0.059s
?   	mework/libs/client/workspacefs	[no test files]
--- libs/sandbox ---
ok  	mework/libs/sandbox	22.578s
ok  	mework/libs/sandbox/agent	0.013s
ok  	mework/libs/sandbox/cmd/mework-sandbox	0.074s
?   	mework/libs/sandbox/engine/cloudflare	[no test files]
?   	mework/libs/sandbox/engine/custom	[no test files]
?   	mework/libs/sandbox/engine/docker	[no test files]
ok  	mework/libs/sandbox/engine/local	0.027s
ok  	mework/libs/sandbox/runtime	0.009s
--- libs/tests ---
ok  	mework/libs/tests	97.334s
ok  	mework/libs/tests/e2e	1.396s
ok  	mework/libs/tests/integration	0.030s
--- libs/tools ---
ok  	mework/libs/tools/import-guard	0.016s
```
