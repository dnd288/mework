# Test results: c0002-repo-restructure

```
$ for mod in shared server client sandbox tests tools; do echo "--- $mod ---"; (cd $mod && go test -p 1 -count=1 ./... 2>&1); echo; done
--- shared ---
ok  	mework/shared	1.117s
?   	mework/shared/config	[no test files]
ok  	mework/shared/core	0.634s
?   	mework/shared/errors	[no test files]
?   	mework/shared/grant	[no test files]
?   	mework/shared/log	[no test files]
ok  	mework/shared/plugin	0.623s
ok  	mework/shared/ports	0.600s
ok  	mework/shared/providers/mello	0.946s
ok  	mework/shared/transport	0.868s

--- server ---
ok  	mework/server	1.774s
?   	mework/server/audit	[no test files]
ok  	mework/server/auth	0.947s
?   	mework/server/bus	[no test files]
?   	mework/server/bus/memory	[no test files]
?   	mework/server/bus/nats	[no test files]
?   	mework/server/bus/postgres	[no test files]
ok  	mework/server/catalog	0.990s
?   	mework/server/cmd/mework-server	[no test files]
ok  	mework/server/connection	0.922s
ok  	mework/server/hub	0.869s
ok  	mework/server/middleware	1.001s
?   	mework/server/notify	[no test files]
ok  	mework/server/orchestrator	1.004s
?   	mework/server/permission	[no test files]
ok  	mework/server/platform/secret	0.726s
ok  	mework/server/platform/store	0.967s
ok  	mework/server/platform/token	0.726s
ok  	mework/server/provider	0.668s
?   	mework/server/provider/github	[no test files]
?   	mework/server/provider/jira	[no test files]
ok  	mework/server/provider/mello	0.783s
?   	mework/server/quota	[no test files]
ok  	mework/server/registry	0.972s
?   	mework/server/scheduler	[no test files]
?   	mework/server/session	[no test files]
?   	mework/server/storage	[no test files]
?   	mework/server/storage/fs	[no test files]
?   	mework/server/storage/minio	[no test files]
?   	mework/server/storage/r2	[no test files]
?   	mework/server/storage/s3	[no test files]
ok  	mework/server/webhook	1.081s
ok  	mework/server/writeback	0.952s

--- client ---
ok  	mework/client	1.344s
ok  	mework/client/cli	0.990s
?   	mework/client/cmd/mework	[no test files]
?   	mework/client/osproc	[no test files]
ok  	mework/client/runner	0.783s
ok  	mework/client/subscribe	0.850s
?   	mework/client/workspacefs	[no test files]

--- sandbox ---
ok  	mework/sandbox	1.323s
ok  	mework/sandbox/agent	0.867s
?   	mework/sandbox/cmd/mework-sandbox	[no test files]
?   	mework/sandbox/engine/cloudflare	[no test files]
?   	mework/sandbox/engine/custom	[no test files]
?   	mework/sandbox/engine/docker	[no test files]
ok  	mework/sandbox/engine/local	0.751s
?   	mework/sandbox/runtime	[no test files]

--- tests ---
ok  	mework/tests	2.556s
ok  	mework/tests/e2e	1.016s

--- tools ---
ok  	mework/tools/import-guard	0.626s
```

All packages pass. Note: DB-backed tests (e.g. `server/platform/store`) skip automatically when `TEST_DATABASE_URL` is not set.
