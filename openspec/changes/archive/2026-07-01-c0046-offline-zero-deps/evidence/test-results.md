# c0046-offline-zero-deps — Test results

```
$ go test -p 1 ./... 2>&1 | tail -40
ok  	mework/apps/mew	0.245s
ok  	mework/apps/mework-mezon-worker	0.318s
ok  	mework/apps/mework-server	0.402s
?   	mework/libs/client/cli	[no test files]
?   	mework/libs/client/enroll	[no test files]
?   	mework/libs/client/subscribe	[no test files]
ok  	mework/libs/client/runner	0.156s
?   	mework/libs/client/catalog	[no test files]
?   	mework/libs/client/workspacefs	[no test files]
--- SKIP: libs/server/platform/store (0.001s)
    store_test.go:42: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/auth (0.001s)
    auth_test.go:28: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/orchestrator (0.001s)
    state_test.go:35: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/webhook (0.001s)
    parse_test.go:22: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/hub (0.001s)
    hub_test.go:30: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/connection (0.001s)
    connection_test.go:25: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/provider (0.001s)
    provider_test.go:20: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/registry (0.001s)
    registry_test.go:33: SKIP: TEST_DATABASE_URL not set
--- SKIP: libs/server/bus (0.001s)
    bus_test.go:18: SKIP: TEST_DATABASE_URL not set
?   	mework/libs/shared/core	[no test files]
?   	mework/libs/shared/transport	[no test files]
?   	mework/libs/shared/config	[no test files]
ok  	mework/libs/tests/integration	0.512s
--- SKIP: libs/tests/e2e (0.001s)
    e2e_test.go:26: SKIP: TEST_DATABASE_URL not set
ok  	mework/libs/sandbox	0.089s
ok  	mework/libs/tools	0.134s
PASS
ok  	github.com/alicebob/miniredis/v2	0.015s
```

DB-backed tests skip when `TEST_DATABASE_URL` is unset. All other tests pass.
