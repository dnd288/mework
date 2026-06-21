// Package bus defines the server-internal message bus interface and domain
// types for event publishing and subscription.
package bus

import (
	"context"
	"errors"
)

// Sentinel errors for delivery acknowledgement.
var (
	ErrMessageNotFound = errors.New("message not found")
	ErrAlreadyAcked    = errors.New("message already acknowledged")
)

// Topic is a hierarchical dot-separated topic name (e.g. "runner.R.dispatch").
type Topic string

// Identity identifies a subscriber (e.g. a runtime or session).
type Identity string

// Filter is a topic pattern supporting single-segment wildcard (*).
// Examples: "runner.*.dispatch", "session.s1.*", "topic.*"
type Filter string

// Message is the payload carried by an event.
type Message struct {
	Payload []byte
}

// Event is a delivered message on a topic with metadata.
type Event struct {
	// ID is a monotonic identifier for ordering and resume.
	ID string
	// Topic is the topic this event was published to.
	Topic Topic
	// Message is the event payload.
	Message Message
}

// Broker is the pluggable message bus interface.
type Broker interface {
	// Publish sends a message on a topic. The message is delivered to existing
	// matching subscribers and retained for future matching subscribers.
	Publish(ctx context.Context, topic Topic, msg Message) error
	// Subscribe opens a subscription. Events matching the filter are delivered
	// on the returned subscription's Events channel. fromEventID, if non-empty,
	// requests delivery of events with an ID greater than the given value
	// (for resume after disconnection).
	Subscribe(ctx context.Context, who Identity, filter Filter, fromEventID string) (Subscription, error)
	// Ack acknowledges a message by its event ID, preventing future redelivery.
	Ack(ctx context.Context, msgID string) error
}

// Subscription represents an active subscription to a topic filter.
type Subscription interface {
	// Events returns a channel that delivers matching events.
	Events() <-chan Event
	// Close terminates the subscription.
	Close() error
}
