package runner

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"mework/libs/server/bus"
	"mework/libs/server/session"
	"mework/libs/shared/core"
)

// TurnResult is the raw outcome of running one chat turn against the backend:
// the backend's combined stdout/stderr and whether it failed.
type TurnResult struct {
	// Output is the backend's combined stdout/stderr for the turn.
	Output string
	// Failed is true when the backend exited with an error for the turn.
	Failed bool
}

// EventPublisher parses a turn's backend output into ChatEvents and publishes
// them to the session control topic on the bus. Late subscribers get the
// retained tail then continue live, courtesy of the broker's resumable delivery.
type EventPublisher struct {
	broker    bus.Broker
	sessionID core.SessionID
	topic     bus.Topic
}

// NewEventPublisher returns an EventPublisher bound to the given session's
// control topic.
func NewEventPublisher(broker bus.Broker, sessionID core.SessionID) *EventPublisher {
	return &EventPublisher{
		broker:    broker,
		sessionID: sessionID,
		topic:     bus.FormatTopic(bus.TopicSessionControl, string(sessionID)),
	}
}

// PublishTurn turns one turn's backend output into a stream of ChatEvents and
// publishes them in order: zero or more token/message events, then exactly one
// terminal event (done on success, error on failure). Unparseable output is
// surfaced as a raw message event rather than being dropped.
func (p *EventPublisher) PublishTurn(ctx context.Context, turn TurnResult) error {
	if turn.Failed {
		// An errored turn emits a single error terminal carrying the output.
		return p.publish(ctx, session.ChatEvent{Kind: session.EventError, Content: turn.Output})
	}

	content := turn.Output
	if !utf8.ValidString(content) {
		// Parse fallback: unparseable output survives as a raw message event so
		// it is never silently dropped, then the turn still terminates with done.
		content = strings.ToValidUTF8(content, "")
	}

	if content != "" {
		if err := p.publish(ctx, session.ChatEvent{Kind: session.EventToken, Content: content}); err != nil {
			return err
		}
		if err := p.publish(ctx, session.ChatEvent{Kind: session.EventMessage, Content: content}); err != nil {
			return err
		}
	}
	return p.publish(ctx, session.ChatEvent{Kind: session.EventDone})
}

func (p *EventPublisher) publish(ctx context.Context, ev session.ChatEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return p.broker.Publish(ctx, p.topic, bus.Message{Payload: payload})
}

// DecodeChatEvent decodes a ChatEvent from a published payload.
func DecodeChatEvent(payload []byte) (session.ChatEvent, error) {
	var ev session.ChatEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return session.ChatEvent{}, err
	}
	return ev, nil
}
