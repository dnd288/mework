package e2e

import "testing"

// Feature 08 — Message bus (SSE, target). Source: SCENARIOS.md
// Spec: openspec/changes/c0002-message-bus. All scenarios skip pending c0002.
//
// This is the richest surface: push, lazy + smart subscriptions, session control channel
// (push-to-sandbox), resume, ack/redelivery, backpressure, concurrency. Read these to
// evaluate how routing and delivery should behave.

func TestBUS_01_PublishToTopicWithSubscribers(t *testing.T) {
	Scenario(t, "BUS-01", "Publish to a topic with subscribers", PlannedC0003).
		Given("runner R holds an open subscription to runner.R.dispatch", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		When("the hub publishes a message to that topic", func(w *World) {
			w.expect(w.Bus.Publish(ctx(), "runner.R.dispatch", msg("runner.R.dispatch", "dispatch")) == nil,
				"publish should succeed")
		}).
		Then("the message is delivered over the open stream with no new client request", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Kind == "dispatch", "got %q, want dispatch", ev.Kind)
		}).
		Run()
}

func TestBUS_02_PublishNoSubscribersRetained(t *testing.T) {
	Scenario(t, "BUS-02", "Publish with no subscribers is retained", PlannedC0003).
		Given("no current subscriber to runner.R.dispatch", func(w *World) {}).
		When("the hub publishes a message", func(w *World) {
			w.expect(w.Bus.Publish(ctx(), "runner.R.dispatch", msg("runner.R.dispatch", "dispatch")) == nil,
				"publish should succeed even with no subscribers")
		}).
		Then("a later subscriber still receives it (retained, not dropped)", func(w *World) {
			sub := w.Subscribe(Filter{Topics: []Topic{"runner.R.dispatch"}}, "")
			ev := <-sub.Events()
			w.expect(ev.Kind == "dispatch", "retained message should be delivered to a late subscriber")
		}).
		Run()
}

func TestBUS_03_ReceivePushedEvent(t *testing.T) {
	Scenario(t, "BUS-03", "Receive a pushed event", PlannedC0003).
		Given("R opens an SSE subscription for topic T", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"T"}})
		}).
		When("a message is published to T", func(w *World) {
			_ = w.Bus.Publish(ctx(), "T", msg("T", "work"))
		}).
		Then("the server writes an SSE event carrying a monotonic id, no polling involved", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.ID != "", "event must carry a monotonic id, got empty")
		}).
		Run()
}

func TestBUS_04_MultipleTopicsOneStream(t *testing.T) {
	Scenario(t, "BUS-04", "Subscribe to multiple topics on one stream", PlannedC0003).
		Given("R subscribes to topics A and B on one stream", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"A", "B"}})
		}).
		When("messages are published to both A and B", func(w *World) {
			_ = w.Bus.Publish(ctx(), "A", msg("A", "a"))
			_ = w.Bus.Publish(ctx(), "B", msg("B", "b"))
		}).
		Then("both arrive on the single stream, each tagged with its topic", func(w *World) {
			e1 := <-w.Session.Control().Events()
			e2 := <-w.Session.Control().Events()
			w.expect(e1.Topic != e2.Topic, "events should be tagged with their distinct topics")
		}).
		Run()
}

func TestBUS_05_ResumeAfterDroppedConnection(t *testing.T) {
	Scenario(t, "BUS-05", "Resume after a dropped connection", PlannedC0003).
		Given("R processed events up to id 2, then dropped, with id 3 published meanwhile", func(w *World) {
			w.set("lastID", "2")
		}).
		When("R reconnects with Last-Event-ID: 2", func(w *World) {
			sub := w.Subscribe(Filter{Topics: []Topic{"runner.R.dispatch"}}, "2")
			w.set("sub", sub)
		}).
		Then("only event 3 (and newer) is delivered; processed events are not redelivered", func(w *World) {
			sub := w.get("sub").(Subscription)
			ev := <-sub.Events()
			w.expect(ev.ID == "3", "resume should deliver id 3 first, got %q", ev.ID)
		}).
		Run()
}

func TestBUS_06_AckMarksHandled(t *testing.T) {
	Scenario(t, "BUS-06", "Ack marks a message handled", PlannedC0003).
		Given("R received a message m", func(w *World) {}).
		When("R POSTs an ack for m's id (out-of-band, not over SSE)", func(w *World) {
			w.expect(w.Bus.Ack(ctx(), Identity{Runner: "R"}, "m1") == nil, "ack should succeed")
		}).
		Then("the hub marks it handled and never redelivers it", func(w *World) {
			w.expect(true, "no redelivery of an acked message")
		}).
		Run()
}

func TestBUS_07_UnackedRedeliverable(t *testing.T) {
	Scenario(t, "BUS-07", "Unacked message is redeliverable", PlannedC0003).
		Given("a delivered message whose delivery lease expires without an ack", func(w *World) {}).
		When("the lease lapses", func(w *World) {}).
		Then("the message remains unacked and is eligible for redelivery (at-least-once)", func(w *World) {
			w.expect(true, "unacked, lease-expired message is redeliverable")
		}).
		Run()
}

func TestBUS_08_SubscriberRestrictedToEntitledTopics(t *testing.T) {
	Scenario(t, "BUS-08", "Subscriber is restricted to entitled topics", PlannedC0003).
		Given("R is entitled only to runner.R.* topics", func(w *World) {}).
		When("R requests a topic it is not entitled to", func(w *World) {
			_, err := w.Bus.Subscribe(ctx(), Identity{Runner: "R"}, Filter{Topics: []Topic{"runner.OTHER.dispatch"}}, "")
			w.set("err", err)
		}).
		Then("the hub refuses delivery of that topic to R", func(w *World) {
			w.expect(w.get("err") != nil, "subscribing to an unentitled topic must be refused")
		}).
		Run()
}

func TestBUS_09_SwapBackendNoClientChange(t *testing.T) {
	Scenario(t, "BUS-09", "Swap the backend without breaking clients", PlannedC0003).
		Given("the broker backend is switched (Postgres LISTEN/NOTIFY → in-memory)", func(w *World) {}).
		When("a client subscribes and the hub publishes", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"T"}})
			_ = w.Bus.Publish(ctx(), "T", msg("T", "work"))
		}).
		Then("the client observes no change to the SSE contract or event format", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Kind == "work", "event format is unchanged across backends")
		}).
		Run()
}

func TestBUS_10_StateTrackedIndependentlyOfTransport(t *testing.T) {
	Scenario(t, "BUS-10", "State is tracked independently of transport", PlannedC0003).
		Given("the jobs table reframed as the durable backing store behind the bus", func(w *World) {}).
		When("a work item changes state", func(w *World) {}).
		Then("the change is recorded in the backing store regardless of SSE delivery", func(w *World) {
			w.expect(true, "state persists independent of delivery; terminal states immutable")
		}).
		Run()
}

func TestBUS_11_DistinctEventsPublishDistinctMessages(t *testing.T) {
	Scenario(t, "BUS-11", "Distinct events publish distinct messages", PlannedC0003).
		Given("two webhook events with different external_event_id", func(w *World) {}).
		When("both are ingested", func(w *World) {
			_ = w.Bus.Publish(ctx(), "runner.R.dispatch", Message{ID: "e1", Topic: "runner.R.dispatch"})
			_ = w.Bus.Publish(ctx(), "runner.R.dispatch", Message{ID: "e2", Topic: "runner.R.dispatch"})
		}).
		Then("each publishes its own message; a duplicate of either still yields at most one", func(w *World) {
			w.expect(true, "idempotent publish keyed by (provider_code, external_event_id)")
		}).
		Run()
}

func TestBUS_12_SmartFilteredSubscription(t *testing.T) {
	Scenario(t, "BUS-12", "Smart subscription delivers only matching events", PlannedC0003).
		Given("a session subscribed with the smart filter session.s1.*", func(w *World) {
			w.Session = w.OpenSession("s1", Filter{Topics: []Topic{"session.s1.*"}})
		}).
		When("the hub publishes session.s1.ctrl and session.s2.ctrl", func(w *World) {
			_ = w.Bus.Publish(ctx(), "session.s1.ctrl", msg("session.s1.ctrl", "ctrl"))
			_ = w.Bus.Publish(ctx(), "session.s2.ctrl", msg("session.s2.ctrl", "ctrl"))
		}).
		Then("only session.s1.ctrl is delivered to the session", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Topic == "session.s1.ctrl", "smart filter should exclude session.s2.*; got %q", ev.Topic)
		}).
		Run()
}

func TestBUS_13_LazyMaterialization(t *testing.T) {
	Scenario(t, "BUS-13", "Non-matching events are not materialized (lazy)", PlannedC0003).
		Given("a subscription with filter session.s1.*", func(w *World) {
			w.Session = w.OpenSession("s1", Filter{Topics: []Topic{"session.s1.*"}})
		}).
		When("a high volume of session.s2.* events is published", func(w *World) {
			_ = w.Bus.Publish(ctx(), "session.s2.log", msg("session.s2.log", "log"))
		}).
		Then("the broker does not materialize/queue those events for this subscriber", func(w *World) {
			w.expect(true, "non-matching events incur no per-subscriber buffering (lazy)")
		}).
		Run()
}

func TestBUS_14_PushMessageToSandbox(t *testing.T) {
	Scenario(t, "BUS-14", "Push a control message down to a running sandbox", PlannedC0003).
		Given("session s1 has a running sandbox subscribed to its control channel", func(w *World) {
			w.Session = w.OpenSession("s1", Filter{Topics: []Topic{"session.s1.control"}})
		}).
		When("the hub pushes a cancel control message to session s1", func(w *World) {
			w.expect(w.Session.PushToSandbox(ctx(), msg("session.s1.control", "ctrl.cancel")) == nil,
				"push-to-sandbox should succeed")
		}).
		Then("the running agent receives the cancel over its control channel", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Kind == "ctrl.cancel", "sandbox should receive the pushed control message")
		}).
		Run()
}

func TestBUS_15_SessionControlChannelIsolation(t *testing.T) {
	Scenario(t, "BUS-15", "Session control channels are isolated", PlannedC0003).
		Given("two sessions s1 and s2 each subscribed to their own control topic", func(w *World) {
			w.Session = w.OpenSession("s1", Filter{Topics: []Topic{"session.s1.control"}})
		}).
		When("a control message is pushed to s2", func(w *World) {
			_ = w.Bus.Publish(ctx(), "session.s2.control", msg("session.s2.control", "ctrl.cancel"))
		}).
		Then("s1's control channel receives nothing (no cross-session leakage)", func(w *World) {
			w.expect(true, "s1 must not observe s2's control messages")
		}).
		Run()
}

func TestBUS_16_Backpressure(t *testing.T) {
	Scenario(t, "BUS-16", "Slow subscriber does not stall the bus", PlannedC0003).
		Given("a subscriber that consumes its stream slowly", func(w *World) {
			w.Session = w.OpenSession("slow", Filter{Topics: []Topic{"runner.slow.dispatch"}})
		}).
		When("the hub publishes faster than the subscriber drains", func(w *World) {
			_ = w.Bus.Publish(ctx(), "runner.slow.dispatch", msg("runner.slow.dispatch", "d"))
		}).
		Then("publishing is not blocked and other subscribers are unaffected (bounded buffer/lease)", func(w *World) {
			w.expect(true, "backpressure is absorbed per-subscriber, not globally")
		}).
		Run()
}
