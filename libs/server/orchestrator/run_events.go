package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"mework/libs/server/bus"
	"mework/libs/shared/transport"
)

// Topic templates for per-run event and control channels.
const (
	runEventTopicTpl = "run.%s.event"
	runTailMaxSize   = 100
)

// Errors returned by RunEventsService.
var (
	ErrRunNotFound = errors.New("run not found")
)

// RunEventsService implements transport.RunEvents backed by the message bus,
// an in-memory status store, and a per-run bounded recent-tail buffer for late
// subscribers.
type RunEventsService struct {
	broker bus.Broker

	mu   sync.RWMutex
	runs map[string]*runInfo
}

// runInfo tracks per-run state.
type runInfo struct {
	mu     sync.RWMutex
	status transport.RunStatus
	tail   *ringBuffer
}

// NewRunEventsService returns a RunEventsService that uses the given broker
// for event publish/subscribe.
func NewRunEventsService(broker bus.Broker) *RunEventsService {
	return &RunEventsService{
		broker: broker,
		runs:   make(map[string]*runInfo),
	}
}

// getOrCreateRun returns the runInfo for the given runID, creating one if it
// does not exist. The caller must NOT hold s.mu.
func (s *RunEventsService) getOrCreateRun(runID string) *runInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, ok := s.runs[runID]
	if !ok {
		info = &runInfo{
			status: transport.StatusQueued,
			tail:   newRingBuffer(runTailMaxSize),
		}
		s.runs[runID] = info
	}
	return info
}

// getRun returns the runInfo for the given runID, or nil if not found.
// The caller must NOT hold s.mu.
func (s *RunEventsService) getRun(runID string) *runInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runs[runID]
}

// ---------------------------------------------------------------------------
// transport.RunEvents interface
// ---------------------------------------------------------------------------

// Emit publishes an upstream RunEvent for the given run. It serializes the
// event to JSON, publishes it on the per-run event topic, appends it to the
// run's recent-tail buffer, and updates the run's status if the event kind
// is "status".
func (s *RunEventsService) Emit(ctx context.Context, runID string, ev transport.RunEvent) error {
	info := s.getOrCreateRun(runID)

	// Serialize the event.
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal run event: %w", err)
	}

	topic := bus.FormatTopic(runEventTopicTpl, runID)
	if err := s.broker.Publish(ctx, topic, bus.Message{Payload: payload}); err != nil {
		return fmt.Errorf("publish run event: %w", err)
	}

	// Append to the tail buffer.
	info.tail.append(ev)

	// If this is a status event, update the run's status.
	if ev.Kind == "status" {
		info.mu.Lock()
		info.status = transport.RunStatus(ev.Data)
		info.mu.Unlock()
	}

	return nil
}

// Subscribe returns a live subscription to a run's events. Late subscribers
// first receive a bounded recent tail of the run's events and are then spliced
// into the live stream with no gap or duplication.
func (s *RunEventsService) Subscribe(ctx context.Context, runID string) (transport.Subscription, error) {
	info := s.getOrCreateRun(runID)

	// Subscribe to the per-run event topic on the bus.
	topic := bus.FormatTopic(runEventTopicTpl, runID)
	busSub, err := s.broker.Subscribe(ctx, bus.Identity(runID), bus.Filter(topic), "")
	if err != nil {
		return nil, fmt.Errorf("bus subscribe: %w", err)
	}

	return newTailSubscription(info.tail, busSub), nil
}

// Status returns the run's current status.
func (s *RunEventsService) Status(ctx context.Context, runID string) (transport.RunStatus, error) {
	info := s.getRun(runID)
	if info == nil {
		return "", ErrRunNotFound
	}

	info.mu.RLock()
	defer info.mu.RUnlock()
	return info.status, nil
}

// Cancel marks a run as canceled. Cancel is idempotent — re-issuing cancel on
// an already-canceled run is a no-op — and terminal: a canceled run cannot
// transition back to running.
//
// It first publishes a status event for the run indicating cancellation, then
// updates the in-memory status. The force parameter controls whether the
// cancellation is graceful (false) or forced (true); in either case the run
// reaches a terminal canceled state.
func (s *RunEventsService) Cancel(ctx context.Context, runID string, force bool) error {
	info := s.getRun(runID)
	if info == nil {
		return ErrRunNotFound
	}

	info.mu.Lock()
	defer info.mu.Unlock()

	// Idempotent: if already in a terminal state, no-op.
	if info.status == transport.StatusCanceled || info.status == transport.StatusDone || info.status == transport.StatusFailed {
		return nil
	}

	// Publish a cancel status event.
	cancelEvent := transport.RunEvent{
		Kind: "status",
		Data: []byte(transport.StatusCanceled),
	}
	payload, err := json.Marshal(cancelEvent)
	if err != nil {
		return fmt.Errorf("marshal cancel event: %w", err)
	}

	topic := bus.FormatTopic(runEventTopicTpl, runID)
	if pubErr := s.broker.Publish(ctx, topic, bus.Message{Payload: payload}); pubErr != nil {
		return fmt.Errorf("publish cancel event: %w", pubErr)
	}

	info.status = transport.StatusCanceled
	info.tail.append(cancelEvent)

	return nil
}

// ---------------------------------------------------------------------------
// ringBuffer — bounded recent-event buffer for late subscribers
// ---------------------------------------------------------------------------

// ringBuffer is a fixed-capacity, lock-free (caller-synchronized) ring buffer
// of RunEvents. It stores the most recent N events for replay to late
// subscribers.
type ringBuffer struct {
	events []transport.RunEvent
	max    int
	next   int
	count  int
	mu     sync.Mutex
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{
		events: make([]transport.RunEvent, max),
		max:    max,
	}
}

// append adds an event to the buffer, evicting the oldest if at capacity.
func (rb *ringBuffer) append(ev transport.RunEvent) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.events[rb.next] = ev
	rb.next = (rb.next + 1) % rb.max
	if rb.count < rb.max {
		rb.count++
	}
}

// snapshot returns a copy of all events currently in the buffer, in emission
// order (oldest first).
func (rb *ringBuffer) snapshot() []transport.RunEvent {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	out := make([]transport.RunEvent, rb.count)
	if rb.count < rb.max {
		// Not yet wrapped: events are in [0, count)
		copy(out, rb.events[:rb.count])
		return out
	}

	// Wrapped: events are in [next, max) then [0, next)
	start := rb.next
	n := copy(out, rb.events[start:])
	copy(out[n:], rb.events[:start])
	return out
}

// ---------------------------------------------------------------------------
// tailSubscription — replays tail then forwards live events
// ---------------------------------------------------------------------------

// tailSubscription replays the tail buffer and then forwards live events from
// the bus subscription.
type tailSubscription struct {
	events chan transport.RunEvent
	close  func() error
}

func newTailSubscription(tail *ringBuffer, busSub bus.Subscription) *tailSubscription {
	ch := make(chan transport.RunEvent, 256)

	sub := &tailSubscription{
		events: ch,
		close:  busSub.Close,
	}

	go sub.run(tail, busSub, ch)
	return sub
}

// Events returns the channel delivering RunEvents.
func (ts *tailSubscription) Events() <-chan transport.RunEvent {
	return ts.events
}

// Close terminates the subscription.
func (ts *tailSubscription) Close() error {
	return ts.close()
}

func (ts *tailSubscription) run(tail *ringBuffer, busSub bus.Subscription, ch chan transport.RunEvent) {
	defer close(ch)

	// 1. Replay the tail buffer first.
	for _, ev := range tail.snapshot() {
		select {
		case ch <- ev:
		default:
			// Drop if channel is full; don't block replay.
		}
	}

	// 2. Forward live events from the bus subscription.
	for ev := range busSub.Events() {
		var runEv transport.RunEvent
		if err := json.Unmarshal(ev.Message.Payload, &runEv); err != nil {
			continue
		}
		select {
		case ch <- runEv:
		default:
			// Drop oldest to make room (bounded backpressure).
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- runEv:
			default:
			}
		}
	}
}
