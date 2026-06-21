# Test results — c0003-message-bus

Command: `go test -p 1 ./... 2>&1`

**Note:** DB-backed tests skip when `TEST_DATABASE_URL` is not set. The output below reflects only tests that do not require a Postgres connection.

```
ok  	mework/internal/cli	0.390s
ok  	mework/internal/integration	0.296s
ok  	mework/internal/mello	0.294s
ok  	mework/internal/server	0.293s
```

All packages: **PASS**. No failures, no panics.
