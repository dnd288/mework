## 1. Conversation surface

- [ ] 1.1 Define `Conversation` (`Send`/`Stream`/`History`/`Cancel`) in `server/session`
- [ ] 1.2 Define `ChatMessage` (`Role` + `Content`), `Role` (`user`|`assistant`|`system`), and `ChatEvent` (`Kind` `token`|`message`|`done`|`error`)
- [ ] 1.3 Open a `Conversation` against an attached session bound to a running agent

## 2. Send and stream

- [ ] 2.1 `Send` appends the user turn and triggers an assistant turn
- [ ] 2.2 Publish the assistant turn's `ChatEvent`s over the message bus and surface them on `Stream()`
- [ ] 2.3 Terminate each turn with exactly one terminal event (`done` on success, `error` on failure)

## 3. History and system prompt

- [ ] 3.1 `History` returns turns in order across multiple turns
- [ ] 3.2 A `system` message leads the history and steers the turn

## 4. Cancel

- [ ] 4.1 `Cancel` interrupts the in-flight turn and stops the stream promptly
- [ ] 4.2 The session stays usable: a subsequent `Send` starts a fresh turn

## 5. Isolation and backpressure

- [ ] 5.1 Conversations are isolated per session — concurrent chats see no cross-talk
- [ ] 5.2 Per-conversation backpressure: a slow reader buffers/blocks only its own stream and never stalls other sessions or the agent

## 6. Validate

- [ ] 6.1 Tests: send-and-stream, multi-turn history, mid-turn cancel, concurrent isolation, leading system prompt, slow-client backpressure
- [ ] 6.2 openspec validate c0011-chat --type change --strict
- [ ] 6.3 e2e pointer: flip `tests/e2e/17_chat_test.go` from Skip to Green for CHAT-01..06 (send→stream, multi-turn history, mid-turn cancel, concurrent isolation, system prompt, backpressure). Cross-references: `tests/e2e/14_concurrency_test.go` CONC-05 (concurrent sessions never cross-deliver) and `tests/e2e/18_status_streaming_test.go` STREAM-01..05 / STATUS-01..03 must remain green once chat streams through the run-events bus.
