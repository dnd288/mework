# Test results — c0033-daemon-interactive-dispatch

```
$ go test -p 1 -v mework/libs/client/runner/  (session/broker tests)

=== RUN   TestEngine_RoutesSessionDispatch
--- PASS: TestEngine_RoutesSessionDispatch (0.00s)
=== RUN   TestEngine_NonSessionDispatchStaysOneShot
--- PASS: TestEngine_NonSessionDispatchStaysOneShot (0.00s)
=== RUN   TestHTTPBroker_PublishPostsEvent
--- PASS: TestHTTPBroker_PublishPostsEvent (0.00s)
=== RUN   TestHTTPBroker_PublishErrorsOnNon2xx
--- PASS: TestHTTPBroker_PublishErrorsOnNon2xx (0.00s)
=== RUN   TestInteractiveSession_MultiTurnOverOneSandbox
--- PASS: TestInteractiveSession_MultiTurnOverOneSandbox (0.00s)
=== RUN   TestInteractiveSession_TurnContentNotOnCommandLine
--- PASS: TestInteractiveSession_TurnContentNotOnCommandLine (0.00s)
=== RUN   TestInteractiveSession_CancelKeepsSandbox
--- PASS: TestInteractiveSession_CancelKeepsSandbox (0.01s)
=== RUN   TestInteractiveSession_IdleReapAndCloseDestroySandbox
=== RUN   TestInteractiveSession_IdleReapAndCloseDestroySandbox/idle_reap_destroys_sandbox
=== RUN   TestInteractiveSession_IdleReapAndCloseDestroySandbox/explicit_close_destroys_sandbox
--- PASS: TestInteractiveSession_IdleReapAndCloseDestroySandbox (0.05s)
=== RUN   TestInteractiveSession_AuthorizationEnforced
=== RUN   TestInteractiveSession_AuthorizationEnforced/owner_in-tenant_with_grant_is_allowed
=== RUN   TestInteractiveSession_AuthorizationEnforced/non-owner_is_denied
=== RUN   TestInteractiveSession_AuthorizationEnforced/cross-tenant_caller_is_denied
=== RUN   TestInteractiveSession_AuthorizationEnforced/caller_without_grant_is_denied
--- PASS: TestInteractiveSession_AuthorizationEnforced (0.00s)
=== RUN   TestInteractiveSession_WorkspaceBinding
=== RUN   TestInteractiveSession_WorkspaceBinding/workspace_bound:_threaded_into_RunSpec.Workspace
=== RUN   TestInteractiveSession_WorkspaceBinding/no_workspace:_RunSpec.Workspace_is_the_zero_value_(unbound_unchanged)
--- PASS: TestInteractiveSession_WorkspaceBinding (0.00s)
=== RUN   TestSessionInput_ControlClosesAndCancels
--- PASS: TestSessionInput_ControlClosesAndCancels (0.00s)
=== RUN   TestProcessSessionDispatch_OpenAndTurns
--- PASS: TestProcessSessionDispatch_OpenAndTurns (0.01s)
=== RUN   TestSessionEvents_PublishedPerTurn
=== RUN   TestSessionEvents_PublishedPerTurn/token_then_message_then_done
=== RUN   TestSessionEvents_PublishedPerTurn/errored_turn_emits_a_single_error_terminal
=== RUN   TestSessionEvents_PublishedPerTurn/unparseable_output_falls_back_to_a_raw_event
--- PASS: TestSessionEvents_PublishedPerTurn (0.00s)
=== RUN   TestSessionEvents_LateSubscriberTailThenLive
--- PASS: TestSessionEvents_LateSubscriberTailThenLive (0.00s)
=== RUN   TestSessionEvents_StatusAndList
--- PASS: TestSessionEvents_StatusAndList (0.00s)
=== RUN   TestStartWorkspaceSession_ServerMode
--- PASS: TestStartWorkspaceSession_ServerMode (0.00s)
=== RUN   TestStartWorkspaceSession_LocalDirectMode
--- PASS: TestStartWorkspaceSession_LocalDirectMode (0.00s)
=== RUN   TestStartWorkspaceSession_MissingSpawnGrantRejected
=== RUN   TestStartWorkspaceSession_MissingSpawnGrantRejected/grant_without_OpSpawn_is_rejected
=== RUN   TestStartWorkspaceSession_MissingSpawnGrantRejected/empty_grant_is_rejected
--- PASS: TestStartWorkspaceSession_MissingSpawnGrantRejected (0.00s)
=== RUN   TestStartWorkspaceSession_ArtifactsReadableBack
--- PASS: TestStartWorkspaceSession_ArtifactsReadableBack (0.00s)

$ go test -p 1 -count=1 mework/libs/client/... mework/libs/server/... mework/libs/shared/...
ok  	mework/libs/client	0.284s
ok  	mework/libs/client/catalog	0.008s
ok  	mework/libs/client/cli	0.160s
ok  	mework/libs/client/cmd/mework	0.003s
ok  	mework/libs/client/enroll	0.005s
ok  	mework/libs/client/runner	4.412s
ok  	mework/libs/client/subscribe	0.008s
ok  	mework/libs/server	0.484s
ok  	mework/libs/server/auth	0.003s
ok  	mework/libs/server/bus	5.914s
ok  	mework/libs/server/catalog	0.006s
ok  	mework/libs/server/channel	0.003s
ok  	mework/libs/server/connection	0.003s
ok  	mework/libs/server/hub	0.003s
ok  	mework/libs/server/middleware	0.003s
ok  	mework/libs/server/orchestrator	0.003s
ok  	mework/libs/server/platform/secret	0.004s
ok  	mework/libs/server/platform/store	0.003s
ok  	mework/libs/server/platform/token	0.002s
ok  	mework/libs/server/provider	0.002s
ok  	mework/libs/server/provider/mello	0.003s
ok  	mework/libs/server/registry	0.003s
ok  	mework/libs/server/session	0.003s
ok  	mework/libs/server/webhook	0.003s
ok  	mework/libs/server/writeback	0.003s
ok  	mework/libs/shared	0.128s
ok  	mework/libs/shared/core	0.002s
ok  	mework/libs/shared/plugin	0.002s
ok  	mework/libs/shared/ports	0.002s
ok  	mework/libs/shared/providers/mello	0.004s
ok  	mework/libs/shared/transport	0.002s
```
