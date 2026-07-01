# Test results — c0045-mezon-standalone-worker

## Command

```
go test -p 1 -count=1 ./... 2>&1
```

## Output

```
?   	mework/apps/mework	[no test files]
ok  	mework/apps/mework-mezon-worker	17.737s
?   	mework/apps/mework-server	[no test files]
```

Exit code: **0**

All tests pass. DB-backed tests skip automatically because `TEST_DATABASE_URL` is not set in this environment — only the `mework-mezon-worker` package contains unit tests runnable without a database.
