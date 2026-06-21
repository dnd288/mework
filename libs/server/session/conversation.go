// Package session manages user-agent sessions and conversations.
//
// This file defines the chat types and Conversation interface for session-scoped
// interactive chat between an operator and an agent.
package session

import "context"

// Role identifies the author of a chat message.
type Role string

const (
	// RoleUser is a human operator's message.
	RoleUser Role = "user"
	// RoleAssistant is the agent's response message.
	RoleAssistant Role = "assistant"
	// RoleSystem is a system-level steering message.
	RoleSystem Role = "system"
)

// ChatMessage is a single turn in a conversation.
type ChatMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatEventKind is the kind of a streaming assistant event.
type ChatEventKind string

const (
	// EventToken is a partial token of the assistant's response.
	EventToken ChatEventKind = "token"
	// EventMessage is a complete assistant message.
	EventMessage ChatEventKind = "message"
	// EventDone signals successful completion of the turn.
	EventDone ChatEventKind = "done"
	// EventError signals a failure or refusal during the turn.
	EventError ChatEventKind = "error"
)

// ChatEvent is a single event streamed as part of an assistant turn.
// A turn streams zero or more token/message events and terminates with
// exactly one terminal event: done on success or error on failure.
type ChatEvent struct {
	Kind    ChatEventKind `json:"kind"`
	Content string        `json:"content,omitempty"`
}

// Conversation is the interface for a session-scoped chat between an operator
// and an agent. It exposes the ordered history of turns and allows sending new
// turns, streaming the assistant's response events, and cancelling an in-flight
// turn.
type Conversation interface {
	// Send appends the given message as a user (or system) turn and triggers an
	// assistant turn. When the message role is "system" it is recorded as a
	// steering message and, if it is the first turn, leads the history.
	Send(ctx context.Context, msg ChatMessage) error

	// Stream returns a receive-only channel of ChatEvents for the current
	// in-progress assistant turn. The channel delivers zero or more
	// token/message events and terminates with exactly one done or error event.
	// After the terminal event the channel is closed.
	Stream() <-chan ChatEvent

	// History returns all conversation turns in chronological order. The
	// leading system message, if any, is returned as the first element.
	History(ctx context.Context) ([]ChatMessage, error)

	// Cancel interrupts the in-flight assistant turn, if any. The stream is
	// stopped promptly and the session remains usable for subsequent Send calls.
	Cancel(ctx context.Context)
}
