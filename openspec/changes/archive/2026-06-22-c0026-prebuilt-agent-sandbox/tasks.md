## 1. Prebuilt sandbox definition format

- [x] 1.1 Extend the sandbox bundle metadata (`libs/sandbox/schema.go` `SandboxBundleMetadata`) to bind `{engine, backend, image, resource limits}` into a definition
- [x] 1.2 Validate definitions: required name/version, known engine, non-empty backend; container engines require an `image`, `local` ignores it
- [x] 1.3 Publish definitions as agent-catalog artifacts (versioned, immutable; reject republishing an existing version with different content)
- [x] 1.4 Resolve a definition by `name@version` and by a moving pointer (`latest`)
- [x] 1.5 Add bundle tooling support in `libs/sandbox/cmd/mework-sandbox` (pack/push a definition)
- [x] 1.6 Ship in-tree starter definitions: `local-claude` first, then `docker-claude`, `codex-docker`

## 2. Sandbox-as-runner with pluggable engine

- [x] 2.1 Map a definition's `engine` through `libs/sandbox/runtime/manager.go` `NewManagerFor` (default `local`)
- [x] 2.2 Enforce one-agent-per-sandbox for definition-driven runs
- [x] 2.3 Extend `libs/sandbox/agent/detect.go` `DefaultBackends` to recognize new backends (`windows-claude`, `v0`)
- [x] 2.4 Confirm adding an engine/backend needs no schema migration

## 3. Run by reference (one-shot)

- [x] 3.1 Resolve a definition reference → build `core.RunSpec` (carry definition ref + engine/backend/image)
- [x] 3.2 Start the sandbox via its engine and run the agent, feeding content over stdin (never argv)
- [x] 3.3 Reject an unknown reference with a not-found result before starting a sandbox

## 4. Interactive long-lived session (local engine first)

- [x] 4.1 Promote `ports.AgentBackend` (`libs/shared/ports/interfaces.go`) into the execution path, or add an interactive variant over `SandboxDriver.Exec`
- [x] 4.2 In `libs/client/runner`, open a session: start the sandbox once and keep the agent process alive
- [x] 4.3 Deliver each chat turn to the running agent over stdin (reuse `libs/server/session` `Conversation.Send`)
- [x] 4.4 Support cancel/interrupt of an in-flight turn (engine `Signals`) without destroying the sandbox
- [x] 4.5 Wire session lifecycle: create/attach/close + idle reaping; destroy the sandbox on close/reap
- [x] 4.6 Enforce ownership + tenant scoping on create/attach/send/cancel/close (reuse `libs/server/session` + `auth-and-secrets` grants)

## 5. Live logs & observability

- [x] 5.1 Parse backend stdout/stderr into `token|message|done|error` events (exactly one terminal per turn)
- [x] 5.2 Publish session/run events to the session topic on the message bus
- [x] 5.3 Tail-then-live for late subscribers (reuse bus resumable delivery / `last_event_id`)
- [x] 5.4 Expose queryable session status and a tenant-scoped session list

## 6. Pre-baked container images

- [x] 6.1 Container-engine runs use the definition's pinned image with no run-time install step
- [x] 6.2 Provide a pre-baked `docker` image with the agent CLI installed for `docker-claude`
- [x] 6.3 `local` engine ignores the image field

## 7. CLI surface

- [x] 7.1 List available prebuilt definitions and open/inspect a session from the CLI (read-only `agent list` / `session list` style)

## 8. Validation

- [x] 8.1 Table-driven unit tests for definition validation, reference resolution, and one-agent-per-sandbox
- [x] 8.2 Interactive-session test on the `local` engine (multi-turn, cancel, idle reap), mirroring `examples/remote-claude`
- [x] 8.3 Event-stream test: `token`/`message`/terminal ordering + tail-then-live
- [x] 8.4 `make vet` and `make test` green
- [x] 8.5 `openspec validate --change c0026-prebuilt-agent-sandbox --strict` passes
