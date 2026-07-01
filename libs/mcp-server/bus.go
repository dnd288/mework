package main

import (
	"context"
)

// BusBroker defines the message-bus interface the notification handler depends on.
// The production implementation connects to the mework daemon's bus.
// Defined in a non-test file so it is available during production builds.
type BusBroker interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
}
