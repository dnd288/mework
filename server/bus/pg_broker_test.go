package bus_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/server/bus"
	"mework/server/bus/postgres"
	"mework/server/platform/store"
)

// newTestDB connects to the test database, runs migrations, and returns a
// clean pool. Skips the test when TEST_DATABASE_URL is not set.
func newTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres-backed broker test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}

	// Clean existing data from the messages table
	if _, err := pool.Exec(ctx, "DELETE FROM messages"); err != nil {
		t.Fatalf("clean messages: %v", err)
	}

	return pool, func() {
		pool.Close()
		_ = store.RollbackMigrations(dsn)
	}
}

func TestPostgresBroker_Contract(t *testing.T) {
	pool, cleanup := newTestDB(t)
	defer cleanup()

	ctx := context.Background()
	testBrokerContract(t, func(t *testing.T) bus.Broker {
		// Clean messages between subtests to prevent cross-test leakage.
		if _, err := pool.Exec(ctx, "DELETE FROM messages"); err != nil {
			t.Fatalf("clean messages: %v", err)
		}
		b, err := postgres.New(pool)
		if err != nil {
			t.Fatalf("postgres.New: %v", err)
		}
		return b
	})
}

func TestPostgresBroker_ResumeAfterDroppedConnection_BUS05(t *testing.T) {
	pool, cleanup := newTestDB(t)
	defer cleanup()

	b, err := postgres.New(pool)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}

	ctx := context.Background()
	topic := bus.Topic("runner.R.dispatch")

	// Publish messages with monotonic ids
	for i := 0; i < 5; i++ {
		if err := b.Publish(ctx, topic, bus.Message{Payload: []byte("msg")}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Subscribe from eventID "2" — should only receive messages with id > 2
	sub, err := b.Subscribe(ctx, bus.Identity("resumer"), bus.Filter(string(topic)), "2")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	// Collect received events; we expect 3 messages (ids 3, 4, 5)
	var received []bus.Event
	deadline := time.After(time.Second)
	for len(received) < 3 {
		select {
		case evt := <-sub.Events():
			received = append(received, evt)
		case <-deadline:
			break
		}
		if len(received) >= 3 {
			break
		}
	}

	if len(received) == 0 {
		t.Fatal("no events received after resume")
	}
	// All delivered events must have id > "2"
	for _, evt := range received {
		if evt.ID <= "2" {
			t.Errorf("received event with id %q which is <= requested fromEventID %q", evt.ID, "2")
		}
	}
}

func TestPostgresBroker_AckPreventsRedelivery_BUS06(t *testing.T) {
	pool, cleanup := newTestDB(t)
	defer cleanup()

	b, err := postgres.New(pool)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}

	ctx := context.Background()
	topic := bus.Topic("runner.R.dispatch")

	sub, err := b.Subscribe(ctx, bus.Identity("acker"), bus.Filter(string(topic)), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if err := b.Publish(ctx, topic, bus.Message{Payload: []byte("to-ack")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case evt := <-sub.Events():
		if err := b.Ack(ctx, evt.ID); err != nil {
			t.Fatalf("Ack: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery")
	}

	// New subscriber should NOT receive the acked message
	sub2, err := b.Subscribe(ctx, bus.Identity("checker"), bus.Filter(string(topic)), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub2.Close()

	select {
	case <-sub2.Events():
		t.Error("acked message was redelivered")
	case <-time.After(200 * time.Millisecond):
		// good — ack prevented redelivery
	}
}

func TestPostgresBroker_UnackedMessageRedelivery_BUS07(t *testing.T) {
	pool, cleanup := newTestDB(t)
	defer cleanup()

	b, err := postgres.New(pool)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}

	ctx := context.Background()
	topic := bus.Topic("runner.R.dispatch")

	sub, err := b.Subscribe(ctx, bus.Identity("leaser"), bus.Filter(string(topic)), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := b.Publish(ctx, topic, bus.Message{Payload: []byte("lease-me")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Receive but do NOT ack
	select {
	case <-sub.Events():
		// message delivered, not acked
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery")
	}

	// Close subscription, wait for lease expiry, reopen
	sub.Close()

	// After lease expiry the message should be redeliverable
	sub2, err := b.Subscribe(ctx, bus.Identity("releaser"), bus.Filter(string(topic)), "")
	if err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	defer sub2.Close()

	select {
	case evt := <-sub2.Events():
		if string(evt.Message.Payload) != "lease-me" {
			t.Errorf("got payload %q, want %q", string(evt.Message.Payload), "lease-me")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for redelivery after lease expiry")
	}
}

func TestPostgresBroker_PerTopicOrdering_CONC04(t *testing.T) {
	pool, cleanup := newTestDB(t)
	defer cleanup()

	b, err := postgres.New(pool)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}

	ctx := context.Background()
	topic := bus.Topic("ordered.topic")

	sub, err := b.Subscribe(ctx, bus.Identity("order-checker"), bus.Filter(string(topic)), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	// Publish three messages in a known order
	for _, payload := range []string{"first", "second", "third"} {
		if err := b.Publish(ctx, topic, bus.Message{Payload: []byte(payload)}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Collect delivered events in order
	var received []string
	deadline := time.After(time.Second)
	for len(received) < 3 {
		select {
		case evt := <-sub.Events():
			received = append(received, string(evt.Message.Payload))
		case <-deadline:
			t.Fatalf("timed out waiting for ordered messages; got %d of 3: %v", len(received), received)
		}
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(received))
	}
	if received[0] != "first" || received[1] != "second" || received[2] != "third" {
		t.Errorf("per-topic ordering violated: got %v, want [first second third]", received)
	}
}
