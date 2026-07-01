package bus_test

import (
	"context"
	"testing"
	"time"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
)

// TestTopicSessionInput_DirectionIsolation verifies the single-direction-per-topic
// model: a turn published to session.<id>.input reaches an .input subscriber but
// NOT a .control subscriber on the same session, and not another session's .input.
// Delta-spec scenario: "Input and control are isolated and single-direction".
func TestTopicSessionInput_DirectionIsolation(t *testing.T) {
	broker := memory.New()
	ctx := context.Background()

	inputTopic := bus.FormatTopic(bus.TopicSessionInput, "s1")
	controlTopic := bus.FormatTopic(bus.TopicSessionControl, "s1")
	otherInput := bus.FormatTopic(bus.TopicSessionInput, "s2")

	if inputTopic != bus.Topic("session.s1.input") {
		t.Fatalf("bus.FormatTopic(bus.TopicSessionInput, s1) = %q, want session.s1.input", inputTopic)
	}

	inputSub, err := broker.Subscribe(ctx, bus.Identity("runner"), bus.Filter(inputTopic), "")
	if err != nil {
		t.Fatalf("subscribe input: %v", err)
	}
	defer inputSub.Close()

	controlSub, err := broker.Subscribe(ctx, bus.Identity("hub"), bus.Filter(controlTopic), "")
	if err != nil {
		t.Fatalf("subscribe control: %v", err)
	}
	defer controlSub.Close()

	otherSub, err := broker.Subscribe(ctx, bus.Identity("runner2"), bus.Filter(otherInput), "")
	if err != nil {
		t.Fatalf("subscribe other: %v", err)
	}
	defer otherSub.Close()

	if err := broker.Publish(ctx, inputTopic, bus.Message{Payload: []byte("turn")}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The .input subscriber must receive the turn.
	select {
	case ev := <-inputSub.Events():
		if string(ev.Message.Payload) != "turn" {
			t.Fatalf("input got payload %q, want turn", ev.Message.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("input subscriber did not receive the turn")
	}

	// The .control subscriber must NOT receive it (no cross-direction leakage).
	select {
	case ev := <-controlSub.Events():
		t.Fatalf("control subscriber leaked event: %q", ev.Message.Payload)
	case <-time.After(100 * time.Millisecond):
	}

	// Another session's .input subscriber must NOT receive it.
	select {
	case ev := <-otherSub.Events():
		t.Fatalf("other-session subscriber leaked event: %q", ev.Message.Payload)
	case <-time.After(100 * time.Millisecond):
	}
}
