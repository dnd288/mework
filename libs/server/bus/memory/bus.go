// Package memory implements an in-memory Broker for tests and single-process
// deployments. It is safe for concurrent use.
package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"mework/libs/server/bus"
)

// New returns a new in-memory Broker.
func New() bus.Broker {
	return &MemoryBroker{
		subs: make([]*sub, 0),
	}
}

// MemoryBroker is a thread-safe in-memory message broker.
type MemoryBroker struct {
	mu      sync.RWMutex
	msgs    []storedMsg
	nextSeq atomic.Int64
	subs    []*sub
	acked   map[string]bool
}

type storedMsg struct {
	id    string
	topic bus.Topic
	msg   bus.Message
}

type sub struct {
	identity bus.Identity
	filter   bus.Filter
	ch       chan bus.Event
	done     chan struct{}
}

// Publish stores the message and delivers it to all matching subscribers.
func (b *MemoryBroker) Publish(_ context.Context, topic bus.Topic, msg bus.Message) error {
	seq := b.nextSeq.Add(1)
	id := fmt.Sprintf("%d", seq)
	sm := storedMsg{id: id, topic: topic, msg: msg}

	b.mu.Lock()
	b.msgs = append(b.msgs, sm)
	// Deliver to existing matching subscribers.
	evt := bus.Event{ID: id, Topic: topic, Message: msg}
	for _, s := range b.subs {
		if bus.MatchTopic(s.filter, topic) {
			// Skip subscribers that have disconnected.
			select {
			case <-s.done:
				continue
			default:
			}
			select {
			case s.ch <- evt:
			default:
				// Subscriber too slow; drop to avoid blocking the bus.
			}
		}
	}
	b.mu.Unlock()
	return nil
}

// Subscribe creates a subscription matching the given filter, replays retained
// messages that match, and returns the subscription.
func (b *MemoryBroker) Subscribe(_ context.Context, who bus.Identity, filter bus.Filter, fromEventID string) (bus.Subscription, error) {
	s := &sub{
		identity: who,
		filter:   filter,
		ch:       make(chan bus.Event, 256),
		done:     make(chan struct{}),
	}

	b.mu.Lock()
	// Replay retained messages matching the filter.
	for _, sm := range b.msgs {
		if !bus.MatchTopic(filter, sm.topic) {
			continue
		}
		if fromEventID != "" && sm.id <= fromEventID {
			continue
		}
		if b.acked != nil && b.acked[sm.id] {
			continue
		}
		evt := bus.Event{ID: sm.id, Topic: sm.topic, Message: sm.msg}
		select {
		case s.ch <- evt:
		default:
		}
	}
	b.subs = append(b.subs, s)
	b.mu.Unlock()

	return s, nil
}

// Ack marks a message as acknowledged. Returns an error if the message does not
// exist or has already been acknowledged.
func (b *MemoryBroker) Ack(_ context.Context, msgID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if the message exists in the store.
	found := false
	for _, sm := range b.msgs {
		if sm.id == msgID {
			found = true
			break
		}
	}
	if !found {
		return bus.ErrMessageNotFound
	}

	if b.acked == nil {
		b.acked = make(map[string]bool)
	}
	if b.acked[msgID] {
		return bus.ErrAlreadyAcked
	}
	b.acked[msgID] = true
	return nil
}

func (s *sub) Events() <-chan bus.Event {
	return s.ch
}

func (s *sub) Close() error {
	select {
	case <-s.done:
		return nil
	default:
		close(s.done)
		close(s.ch)
	}
	return nil
}
