# Test Results — c0000-tenancy

Command: `go test -p 1 ./... 2>&1 | tail -40`

DB-backed tests skip without `TEST_DATABASE_URL` (set via `make test-db`); the run below was without it, so those tests are skipped, not failed.

```
ok  	mework/cmd/mework	(cached)
?   	mework/cmd/mework-server	[no test files]
ok  	mework/internal/agentrun	(cached)
ok  	mework/internal/cli	(cached)
ok  	mework/internal/daemon	(cached)
ok  	mework/internal/integration	(cached)
ok  	mework/internal/mello	(cached)
ok  	mework/internal/meworkclient	(cached)
ok  	mework/internal/server	(cached)
ok  	mework/internal/server/auth	(cached)
ok  	mework/internal/server/connection	(cached)
ok  	mework/internal/server/jobs	(cached)
ok  	mework/internal/server/middleware	(cached)
ok  	mework/internal/server/profile	(cached)
ok  	mework/internal/server/provider	(cached)
ok  	mework/internal/server/provider/mello	(cached)
ok  	mework/internal/server/registry	(cached)
ok  	mework/internal/server/secret	(cached)
ok  	mework/internal/server/token	(cached)
ok  	mework/internal/server/webhook	(cached)
ok  	mework/internal/store	(cached)
ok  	mework/tests/e2e	(cached)
```
