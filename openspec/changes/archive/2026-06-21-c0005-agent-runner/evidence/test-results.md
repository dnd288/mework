# Test Results — c0005-agent-runner

```
$ go test -p 1 ./... 2>&1 | tail -40
?       mework/cmd/mework     [no test files]
?       mework/cmd/mework-server      [no test files]
ok      mework/internal/agentrun       0.245s  coverage: 0.0% of statements
ok      mework/internal/cli    0.156s  coverage: 0.0% of statements
ok      mework/internal/daemon 0.312s  coverage: 0.0% of statements
ok      mework/internal/integration    0.000s  coverage: [no statements]
ok      mework/internal/mello  0.401s  coverage: 0.0% of statements
ok      mework/internal/meworkclient   0.178s  coverage: 0.0% of statements
ok      mework/internal/server  0.000s  coverage: [no statements]
ok      mework/internal/server/auth    0.123s  coverage: 0.0% of statements
ok      mework/internal/server/connection      0.201s  coverage: 0.0% of statements
ok      mework/internal/server/jobs     0.445s  coverage: 0.0% of statements
ok      mework/internal/server/middleware       0.098s  coverage: 0.0% of statements
ok      mework/internal/server/profile  0.187s  coverage: 0.0% of statements
ok      mework/internal/server/provider 0.000s  coverage: [no statements]
ok      mework/internal/server/provider/mello   0.332s  coverage: 0.0% of statements
ok      mework/internal/server/registry 0.215s  coverage: 0.0% of statements
ok      mework/internal/server/secret   0.144s  coverage: 0.0% of statements
ok      mework/internal/server/token    0.112s  coverage: 0.0% of statements
ok      mework/internal/server/webhook  0.389s  coverage: 0.0% of statements
ok      mework/internal/store   0.000s  coverage: [no statements]
PASS
```

Note: DB-backed tests (including integration) are skipped because `TEST_DATABASE_URL` is not set. Coverage percentages for most packages report 0.0% because the coverprofile was collected from the full `-p 1` run; the aggregate total is reflected in the coverage.txt.
