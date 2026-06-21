package bus_test

import (
	"context"
	"testing"
	"time"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
)

// testBrokerContract runs the standard Broker contract assertions against any
// implementation. Each subtest gets its own broker instance so tests are
// isolated. Called from both the in-memory and Postgres test suites.
func testBrokerContract(t *testing.T, newBroker func(t *testing.T) bus.Broker) {
	t.Helper()

	tests := []struct {
		name string
		run  func(t *testing.T, b bus.Broker)
	}{
		{
			name: "publish to a topic with subscriber",
			run: func(t *testing.T, b bus.Broker) {
				ctx := context.Background()
				sub, err := b.Subscribe(ctx, bus.Identity("test-1"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				if err := b.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("hello")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}

				select {
				case evt := <-sub.Events():
					if string(evt.Message.Payload) != "hello" {
						t.Errorf("got payload %q, want %q", string(evt.Message.Payload), "hello")
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for published message")
				}
			},
		},
		{
			name: "published message is retained for future subscribers BUS-02",
			run: func(t *testing.T, b bus.Broker) {
				ctx := context.Background()
				// Publish before anyone subscribes
				if err := b.Publish(ctx, bus.Topic("runner.R.dispatch"), bus.Message{Payload: []byte("retained")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}
				// Now subscribe — should receive the retained message
				sub, err := b.Subscribe(ctx, bus.Identity("test-2"), bus.Filter("runner.R.dispatch"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				select {
				case evt := <-sub.Events():
					if string(evt.Message.Payload) != "retained" {
						t.Errorf("got payload %q, want %q", string(evt.Message.Payload), "retained")
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for retained message")
				}
			},
		},
		{
			name: "subscribe to multiple topics on one stream BUS-04",
			run: func(t *testing.T, b bus.Broker) {
				ctx := context.Background()
				sub, err := b.Subscribe(ctx, bus.Identity("multi"), bus.Filter("topic.*"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				if err := b.Publish(ctx, bus.Topic("topic.A"), bus.Message{Payload: []byte("msg-a")}); err != nil {
					t.Fatalf("Publish A: %v", err)
				}
				if err := b.Publish(ctx, bus.Topic("topic.B"), bus.Message{Payload: []byte("msg-b")}); err != nil {
					t.Fatalf("Publish B: %v", err)
				}

				seen := make(map[string]bool)
				deadline := time.After(time.Second)
				for remaining := 2; remaining > 0; {
					select {
					case evt := <-sub.Events():
						seen[string(evt.Topic)] = true
						remaining--
					case <-deadline:
						t.Fatalf("timed out waiting for events; seen %v", seen)
					}
				}
				if !seen["topic.A"] || !seen["topic.B"] {
					t.Errorf("expected both topics, got %v", seen)
				}
			},
		},
		{
			name: "smart filter delivers only matching events BUS-12",
			run: func(t *testing.T, b bus.Broker) {
				ctx := context.Background()
				sub, err := b.Subscribe(ctx, bus.Identity("s1"), bus.Filter("session.s1.*"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				if err := b.Publish(ctx, bus.Topic("session.s1.ctrl"), bus.Message{Payload: []byte("s1-msg")}); err != nil {
					t.Fatalf("Publish s1: %v", err)
				}
				if err := b.Publish(ctx, bus.Topic("session.s2.ctrl"), bus.Message{Payload: []byte("s2-msg")}); err != nil {
					t.Fatalf("Publish s2: %v", err)
				}

				select {
				case evt := <-sub.Events():
					if string(evt.Topic) != "session.s1.ctrl" {
						t.Errorf("expected topic session.s1.ctrl, got %s", string(evt.Topic))
					}
					if string(evt.Message.Payload) != "s1-msg" {
						t.Errorf("expected payload s1-msg, got %s", string(evt.Message.Payload))
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for matching event")
				}

				select {
				case <-sub.Events():
					t.Error("received unexpected second event (s2 leaked)")
				case <-time.After(200 * time.Millisecond):
					// good — no second event
				}
			},
		},
		{
			name: "lazy delivery — non-matching traffic not materialized BUS-13",
			run: func(t *testing.T, b bus.Broker) {
				ctx := context.Background()
				sub, err := b.Subscribe(ctx, bus.Identity("s1"), bus.Filter("session.s1.*"), "")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}
				defer sub.Close()

				for i := 0; i < 100; i++ {
					if err := b.Publish(ctx, bus.Topic("session.s2.log"), bus.Message{Payload: []byte("noise")}); err != nil {
						t.Fatalf("Publish: %v", err)
					}
				}

				select {
				case <-sub.Events():
					t.Error("received event for non-matching traffic")
				case <-time.After(200 * time.Millisecond):
					// good — lazy delivery works
				}
			},
		},
		{
			name: "control channel isolation BUS-15",
			run: func(t *testing.T, b bus.Broker) {
				ctx := context.Background()
				sub1, err := b.Subscribe(ctx, bus.Identity("s1"), bus.Filter("session.s1.control"), "")
				if err != nil {
					t.Fatalf("Subscribe s1: %v", err)
				}
				defer sub1.Close()

				// Publish to s2.control — s1 must not see it
				if err := b.Publish(ctx, bus.Topic("session.s2.control"), bus.Message{Payload: []byte("s2-only")}); err != nil {
					t.Fatalf("Publish: %v", err)
				}

				select {
				case <-sub1.Events():
					t.Error("s1 received s2 control message")
				case <-time.After(200 * time.Millisecond):
					// good — control isolation works
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBroker(t)
			tt.run(t, b)
		})
	}
}

func TestInMemoryBroker_Contract(t *testing.T) {
	testBrokerContract(t, func(t *testing.T) bus.Broker {
		return memory.New()
	})
}
