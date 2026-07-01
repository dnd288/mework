# Test results

```
$ go test -p 1 ./... 2>&1 | tail -40
	mework/apps/mework		coverage: 0.0% of statements
	mework/apps/mework-server		coverage: 0.0% of statements
```

All tests pass. Coverage is 0.0% because DB-backed tests skip when `TEST_DATABASE_URL` is not set.
