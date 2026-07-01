## 1. Enrollment

- [x] 1.1 Hub: issue short-lived, single-use registration tokens; exchange endpoint that returns a durable runner credential
- [x] 1.2 Client: `mework runner enroll --url --token` — exchange and persist identity at `~/.mework/` (0600)
- [x] 1.3 Reject invalid/expired registration tokens; never persist identity on failure

## 2. Subscription & presence

- [x] 2.1 Open the SSE subscription to the runner's topics on `daemon start` (via `message-bus` client)
- [x] 2.2 Maintain presence/heartbeat over the channel; surface online/offline to the hub
- [x] 2.3 Reconnect with jittered backoff and `Last-Event-ID` resume

## 3. Pull-run-report loop

- [x] 3.1 On dispatch: resolve + pull the referenced agent version from the catalog
- [x] 3.2 Run it via the sandbox runtime (handoff to `sandbox-runtime`)
- [x] 3.3 Report terminal result (done/failed + summary) over POST; acknowledge the dispatch

## 4. Grant enforcement

- [x] 4.1 Parse and verify the grant carried by the dispatch (integrity-checked)
- [x] 4.2 Refuse operations outside the grant locally; report refusals

## 5. CLI / daemon reshape

- [x] 5.1 Add `runner enroll`, `agent list`, `session list` commands in `client/cli`
- [x] 5.2 Remove the poll loop and claim path from `client/runner` (was `client/runner`) and `client/subscribe` (was `client/subscribe`)
- [x] 5.3 Provide a migration/compat path for existing registered runtimes

## 6. Validation

- [x] 6.1 Tests: enroll → restart → unattended resume; dispatch → pull → run → report → ack; grant refusal
- [x] 6.2 `openspec validate --change agent-runner --strict`
- [x] 6.3 e2e pointer: flip `tests/e2e/04_runner_enroll_test.go` from Skip to Green for ENROLL-01..05; flip `tests/e2e/10_runner_loop_test.go` from Skip to Green for LOOP-01..09; flip `tests/e2e/14_concurrency_test.go` from Skip to Green for CONC-01 (concurrent dispatches all delivered) and CONC-02 (one agent at a time per runner). The MODIFIED daemon-runtime requirement (LOOP-02 "no interval polling when idle") is exercised by asserting no `POST /jobs/claim` traffic in `tests/e2e/05_daemon_test.go` while idle.
