# Test results -- c0042-orchestrator-mcp

`go test -p 1 ./...` (workspace mode) -- all tests passing, zero failures.

Note: DB-backed integration tests (`mework/libs/tests/integration`) require
`TEST_DATABASE_URL` and are skipped when it is not set; they passed when
configured.

```
?   	mework/apps/mework	[no test files]
?   	mework/apps/mework-server	[no test files]
ok  	mework/client	0.410s
ok  	mework/client/catalog	0.380s
ok  	mework/client/cli	1.204s
ok  	mework/client/cmd/mework	0.328s
ok  	mework/client/enroll	0.381s
ok  	mework/client/runner	1.086s
ok  	mework/client/subscribe	0.484s
ok  	mework/client/workspacefs	0.351s
?   	mework/examples/mello-claude	[no test files]
ok  	mework/libs/client	0.339s
ok  	mework/libs/mcp-server	0.860s
ok  	mework/libs/sandbox	0.637s
ok  	mework/libs/server	0.827s
ok  	mework/libs/shared	0.497s
?   	mework/libs/tests	[no test files]
ok  	mework/libs/tests/integration	0.364s
ok  	mework/libs/tools/import-guard	0.371s
ok  	mework/sandbox	0.563s
ok  	mework/sandbox/agent	0.352s
ok  	mework/sandbox/cmd/mework-sandbox	0.316s
?   	mework/sandbox/engine/cloudflare	[no test files]
?   	mework/sandbox/engine/custom	[no test files]
?   	mework/sandbox/engine/docker	[no test files]
ok  	mework/sandbox/engine/local	0.546s
ok  	mework/sandbox/runtime	0.399s
ok  	mework/server	0.868s
?   	mework/server/audit	[no test files]
ok  	mework/server/auth	0.444s
ok  	mework/server/bus	6.553s
?   	mework/server/bus/memory	[no test files]
?   	mework/server/bus/nats	[no test files]
?   	mework/server/bus/postgres	[no test files]
ok  	mework/server/catalog	0.406s
ok  	mework/server/channel	0.401s
?   	mework/server/cmd/mework-server	[no test files]
ok  	mework/server/connection	0.535s
ok  	mework/server/hub	0.387s
ok  	mework/server/middleware	0.378s
?   	mework/server/notify	[no test files]
ok  	mework/server/orchestrator	0.394s
?   	mework/server/permission	[no test files]
ok  	mework/server/platform/secret	0.426s
ok  	mework/server/platform/store	0.387s
ok  	mework/server/platform/token	0.358s
ok  	mework/server/provider	0.393s
?   	mework/server/provider/github	[no test files]
?   	mework/server/provider/jira	[no test files]
ok  	mework/server/provider/mello	0.362s
?   	mework/server/quota	[no test files]
ok  	mework/server/registry	0.380s
?   	mework/server/scheduler	[no test files]
ok  	mework/server/session	0.565s
?   	mework/server/storage	[no test files]
?   	mework/server/storage/fs	[no test files]
?   	mework/server/storage/minio	[no test files]
?   	mework/server/storage/r2	[no test files]
?   	mework/server/storage/s3	[no test files]
?   	mework/server/storage/s3compat	[no test files]
ok  	mework/server/webhook	0.394s
ok  	mework/server/writeback	0.458s
ok  	mework/shared	0.452s
?   	mework/shared/config	[no test files]
ok  	mework/shared/core	0.356s
?   	mework/shared/errors	[no test files]
?   	mework/shared/grant	[no test files]
?   	mework/shared/log	[no test files]
ok  	mework/shared/plugin	0.381s
ok  	mework/shared/ports	0.367s
ok  	mework/shared/providers/mello	0.388s
ok  	mework/shared/transport	0.346s
```

All packages with test files reported `ok`. Zero failures.
