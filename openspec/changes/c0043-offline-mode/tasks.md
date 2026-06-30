## 1. Offline Agent IPC — Unix socket listener

- [ ] 1.1 Create `libs/client/runner/offline.go` — the offline agent lifecycle: listen on a Unix socket at `/tmp/mework-offline-<workspace-hash>.sock`, accept JSON-RPC messages, dispatch tasks to the sandbox, stream responses
- [ ] 1.2 Create `libs/client/runner/offline_client.go` — the `mework run` IPC client: connect to the socket, send instruction, stream response back to stdout
- [ ] 1.3 Derive socket path from SHA-256 hash of the resolved workspace directory path, making it unique per workspace
- [ ] 1.4 Unlink socket on agent startup and clean up on graceful shutdown (SIGINT/SIGTERM)

## 2. CLI — daemon start --offline flag

- [x] 2.1 Add `--offline` and `--workspace` flags to `mework daemon start` command in `libs/client/cli/daemon.go`
- [x] 2.2 When `--offline` is set, skip hub enrollment/connection and use `workspace_start.StartWorkspaceSession` instead of the SSE dispatch loop
- [x] 2.3 Validate workspace directory exists and contains a valid `mework.yml` at startup
- [x] 2.4 Validate the `engine` field is `local` (reject docker/cloudflare in offline mode with a clear error)
- [x] 2.5 Start the offline IPC listener after the sandbox is ready

## 3. CLI — mework run command

- [x] 3.1 Create `libs/client/cli/cmd_run.go` — `mework run <instruction>` command that connects to the running offline agent's Unix socket, sends the instruction, and streams the response to stdout
- [x] 3.2 Use stdin (not argv) to pass the instruction to the sandbox, preserving the injection-safety invariant
- [x] 3.3 Handle agent-not-running case: error if socket doesn't exist
- [x] 3.4 Handle agent error case: propagate non-zero exit codes
- [x] 3.5 Write PID file to `~/.mework/offline.pid` for `mework stop` to find

## 4. Wire into CLI registration

- [x] 4.1 Add `runCmd` to `registerCommands()` in `libs/client/cli/help.go`
- [x] 4.2 Ensure `mework daemon start --offline` and `mework daemon stop` cover the offline agent lifecycle

## 5. Tests

- [ ] 5.1 Unit tests for offline IPC: start agent, send instruction, verify response
- [x] 5.2 Unit test: `mework start --offline` without `--workspace` prints error
- [x] 5.3 Unit test: `mework start --workspace <bad-dir> --offline` prints error
- [x] 5.4 Unit test: `mework run` with no running agent prints error
- [ ] 5.5 Unit test: offline agent resolves definition from workspace mework.yml
- [ ] 5.6 Unit test: offline agent rejects non-local engine in mework.yml
