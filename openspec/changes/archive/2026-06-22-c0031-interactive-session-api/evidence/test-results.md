# Test Results — c0031-interactive-session-api

Generated: 2026-06-22T05:51:02Z  |  Toolchain: go version go1.26.4 linux/amd64

## New c0031 tests (verbose)
```
=== RUN   TestDispatch_OwnerTenantRoundTrip
--- PASS: TestDispatch_OwnerTenantRoundTrip (0.00s)
PASS
ok  	mework/libs/shared/transport	0.002s
=== RUN   TestDispatchSessionToRunner
--- PASS: TestDispatchSessionToRunner (0.00s)
=== RUN   TestDispatchSessionToRunner_NilGrant
--- PASS: TestDispatchSessionToRunner_NilGrant (0.00s)
PASS
ok  	mework/libs/server/catalog	0.003s
=== RUN   TestCreateSession_DispatchesAndUsesAuthContext
--- PASS: TestCreateSession_DispatchesAndUsesAuthContext (0.00s)
=== RUN   TestListSessions_TenantScoped
--- PASS: TestListSessions_TenantScoped (0.00s)
=== RUN   TestGetAndCloseSession
--- PASS: TestGetAndCloseSession (0.00s)
=== RUN   TestResultSession_204
--- PASS: TestResultSession_204 (0.00s)
PASS
ok  	mework/libs/server/session	0.003s
=== RUN   TestSessionRoutes_Lifecycle
--- PASS: TestSessionRoutes_Lifecycle (0.88s)
PASS
ok  	mework/libs/server/hub	0.884s
```

## Touched-package summary
```
ok  	mework/libs/shared/transport	(cached)
ok  	mework/libs/server/catalog	(cached)
ok  	mework/libs/server/session	(cached)
ok  	mework/libs/server/hub	(cached)
ok  	mework/libs/client/runner	(cached)
```
