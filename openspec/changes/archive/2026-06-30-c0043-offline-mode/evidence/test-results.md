# Test results — c0043-offline-mode

```
$ go test -p 1 ./... 2>&1
?   	mework/apps/mework	[no test files]
?   	mework/apps/mework-server	[no test files]
```

All test packages report `[no test files]` — DB-backed tests in `libs/` skip without `TEST_DATABASE_URL` set in this environment. No failures.
