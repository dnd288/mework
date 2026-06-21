// Package transport holds the wire contracts — the SSE event schema, API DTOs,
// and the runner↔sandbox protocol. These are shared across all mework
// components so each contract has one source of truth.
package transport

// SSEEvent is a server-sent event sent from the server to subscribed clients.
type SSEEvent struct {
	Type string
	Data []byte
}

// TODO: API DTOs and runner↔sandbox protocol types will be added here as
// the redesign fills in the wire contracts.
