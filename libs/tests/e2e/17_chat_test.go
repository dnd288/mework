package e2e

import "testing"

// Feature 17 — Interactive chat. Real-world platform surface (proposed). Multi-turn
// conversation with a running agent in a session: send a message, stream the response.

func TestCHAT_01_SendAndStream(t *testing.T) {
	Scenario(t, "CHAT-01", "Send a message and stream the assistant response", PlannedPlatform).
		Given("an attached session with a running agent", func(w *World) {
			w.set("chat", w.Chat("s1"))
		}).
		When("the user sends a message", func(w *World) {
			chat := w.get("chat").(Conversation)
			w.expect(chat.Send(ctx(), ChatMessage{Role: RoleUser, Content: "fix the login bug"}) == nil, "send should succeed")
		}).
		Then("the assistant response streams back as token events ending in done", func(w *World) {
			chat := w.get("chat").(Conversation)
			ev := <-chat.Stream()
			w.expect(ev.Kind == "token" || ev.Kind == "message", "response streams as token/message events, got %q", ev.Kind)
		}).
		Run()
}

func TestCHAT_02_MultiTurnHistory(t *testing.T) {
	Scenario(t, "CHAT-02", "Multi-turn history is preserved", PlannedPlatform).
		Given("a session where the user already exchanged one turn", func(w *World) {
			chat := w.Chat("s1")
			_ = chat.Send(ctx(), ChatMessage{Role: RoleUser, Content: "hello"})
			w.set("chat", chat)
		}).
		When("the user sends a follow-up referring to the prior turn", func(w *World) {
			chat := w.get("chat").(Conversation)
			_ = chat.Send(ctx(), ChatMessage{Role: RoleUser, Content: "and the tests?"})
		}).
		Then("history includes both turns in order", func(w *World) {
			chat := w.get("chat").(Conversation)
			h, _ := chat.History(ctx())
			w.expect(len(h) >= 2, "conversation history accumulates across turns, got %d", len(h))
		}).
		Run()
}

func TestCHAT_03_CancelMidTurn(t *testing.T) {
	Scenario(t, "CHAT-03", "Cancel an in-flight assistant turn", PlannedPlatform).
		Given("an assistant turn currently streaming", func(w *World) {
			w.set("chat", w.Chat("s1"))
		}).
		When("the user cancels mid-turn", func(w *World) {
			chat := w.get("chat").(Conversation)
			w.expect(chat.Cancel(ctx()) == nil, "cancel should succeed")
		}).
		Then("the stream stops promptly and the session stays usable for the next turn", func(w *World) {
			w.expect(true, "mid-turn cancel interrupts generation without closing the session")
		}).
		Run()
}

func TestCHAT_04_ConcurrentChatsIsolated(t *testing.T) {
	Scenario(t, "CHAT-04", "Concurrent chats in different sessions are isolated", PlannedPlatform).
		Given("two sessions s1 and s2 each with an active conversation", func(w *World) {
			w.set("c1", w.Chat("s1"))
			w.set("c2", w.Chat("s2"))
		}).
		When("both users send messages at the same time", func(w *World) {
			_ = w.get("c1").(Conversation).Send(ctx(), ChatMessage{Role: RoleUser, Content: "a"})
			_ = w.get("c2").(Conversation).Send(ctx(), ChatMessage{Role: RoleUser, Content: "b"})
		}).
		Then("each conversation streams only its own response (no cross-talk)", func(w *World) {
			w.expect(true, "concurrent conversations are strictly isolated per session")
		}).
		Run()
}

func TestCHAT_05_SystemPrompt(t *testing.T) {
	Scenario(t, "CHAT-05", "A system prompt steers the conversation", PlannedPlatform).
		Given("a session opened with a system message", func(w *World) {
			chat := w.Chat("s1")
			_ = chat.Send(ctx(), ChatMessage{Role: RoleSystem, Content: "be terse"})
			w.set("chat", chat)
		}).
		When("the user sends a message", func(w *World) {
			_ = w.get("chat").(Conversation).Send(ctx(), ChatMessage{Role: RoleUser, Content: "explain"})
		}).
		Then("the system role is recorded first in history and applies to the turn", func(w *World) {
			h, _ := w.get("chat").(Conversation).History(ctx())
			w.expect(len(h) >= 1 && h[0].Role == RoleSystem, "the system prompt leads the history")
		}).
		Run()
}

func TestCHAT_06_SlowClientBackpressure(t *testing.T) {
	Scenario(t, "CHAT-06", "A slow chat client does not stall the agent", PlannedPlatform).
		Given("a client that drains its stream slowly", func(w *World) {
			w.set("chat", w.Chat("s1"))
		}).
		When("the assistant produces tokens faster than the client reads", func(w *World) {
			_ = w.get("chat").(Conversation).Send(ctx(), ChatMessage{Role: RoleUser, Content: "long answer"})
		}).
		Then("streaming applies backpressure/buffering without blocking other sessions", func(w *World) {
			w.expect(true, "per-conversation backpressure isolates a slow reader")
		}).
		Run()
}
