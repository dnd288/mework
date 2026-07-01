// Package postgres implements the Broker interface using PostgreSQL as a
// durable backing store. It combines live delivery via an in-memory subscriber
// list with durable storage and query-based replay for new subscribers.
package postgres

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/bus"
)

// New returns a new Postgres-backed Broker.
func New(pool *pgxpool.Pool) (bus.Broker, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres broker: pool is nil")
	}
	return &PostgresBroker{
		pool: pool,
		subs: make([]*pgSub, 0),
	}, nil
}

// PostgresBroker is a durable message broker backed by PostgreSQL.
type PostgresBroker struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
	subs []*pgSub
}

type pgSub struct {
	identity bus.Identity
	filter   bus.Filter
	ch       chan bus.Event
	done     chan struct{}
}

// Publish inserts the message into the messages table, then delivers it to all
// existing matching subscribers via their event channels.
func (b *PostgresBroker) Publish(ctx context.Context, topic bus.Topic, msg bus.Message) error {
	id, err := insertMessage(ctx, b.pool, string(topic), msg.Payload)
	if err != nil {
		return err
	}

	evt := bus.Event{
		ID:      fmt.Sprintf("%d", id),
		Topic:   topic,
		Message: msg,
	}

	b.mu.RLock()
	for _, s := range b.subs {
		if bus.MatchTopic(s.filter, topic) {
			select {
			case s.ch <- evt:
			default:
				// Subscriber too slow; drop to avoid blocking the bus.
			}
		}
	}
	b.mu.RUnlock()

	return nil
}

// Subscribe queries the database for matching unacked messages, replays them
// to the subscriber, and registers the subscriber for live delivery.
func (b *PostgresBroker) Subscribe(ctx context.Context, who bus.Identity, filter bus.Filter, fromEventID string) (bus.Subscription, error) {
	s := &pgSub{
		identity: who,
		filter:   filter,
		ch:       make(chan bus.Event, 256),
		done:     make(chan struct{}),
	}

	// Replay undelivered messages from the database.
	fromID, err := parseFromID(fromEventID)
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	var rows []messageRow
	if string(filter) == "*" || string(filter) == "**" {
		rows, err = fetchAllUndelivered(ctx, b.pool)
	} else {
		rows, err = fetchUndelivered(ctx, b.pool, filter, fromID)
	}
	if err != nil {
		return nil, fmt.Errorf("subscribe fetch: %w", err)
	}

	for _, row := range rows {
		if !bus.MatchTopic(filter, bus.Topic(row.Topic)) {
			continue
		}
		if fromID > 0 && row.ID <= fromID {
			continue
		}
		evt := bus.Event{
			ID:    fmt.Sprintf("%d", row.ID),
			Topic: bus.Topic(row.Topic),
			Message: bus.Message{
				Payload: row.Payload,
			},
		}
		select {
		case s.ch <- evt:
		default:
		}
	}

	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()

	return s, nil
}

// Ack marks a message as acknowledged in the database.
func (b *PostgresBroker) Ack(ctx context.Context, msgID string) error {
	id, err := parseFromID(msgID)
	if err != nil {
		return fmt.Errorf("ack: %w", err)
	}
	return ackMessage(ctx, b.pool, id)
}

// Events returns the subscription's event channel.
func (s *pgSub) Events() <-chan bus.Event {
	return s.ch
}

// Close terminates the subscription.
func (s *pgSub) Close() error {
	select {
	case <-s.done:
		return nil
	default:
		close(s.done)
		close(s.ch)
	}
	return nil
}
