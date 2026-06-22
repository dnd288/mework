package runner

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"mework/libs/client/subscribe"
	"mework/libs/server/bus"
	"mework/libs/shared/transport"
)

// Engine is the runner's core loop: it subscribes over SSE to its dispatch
// topic, receives dispatches by push (never poll), processes them serially
// via the pull-run-report lifecycle, and maintains presence with heartbeats.
// On stream disconnection it reconnects with jittered backoff and Last-Event-ID
// resume so no dispatch is lost or double-processed.
type Engine struct {
	runnerID   string
	secret     string
	hubURL     string
	catalogURL string
	client     *subscribe.Client
	stream     atomic.Value // holds *subscribe.SSEStream
	dispatchCh chan transport.Dispatch
	eventIDs   chan string // event ID for each dispatch, paired with dispatchCh
	stopCh     chan struct{}
	wg         sync.WaitGroup
	lastEventID atomic.Value // holds string
	closed      atomic.Bool

	// dispatchHook is set in tests to intercept dispatch processing instead of
	// calling processDispatch. The hook receives the dispatch and its SSE event ID.
	// When nil (the normal case), the real lifecycle runs.
	dispatchHook func(d transport.Dispatch, eventID string)
}

// NewEngine creates a new Engine with the given runner identity and hub/catalog
// URLs. The engine is not started; call Start to begin the subscription loop.
func NewEngine(runnerID, secret, hubURL, catalogURL string) *Engine {
	return &Engine{
		runnerID:   runnerID,
		secret:     secret,
		hubURL:     hubURL,
		catalogURL: catalogURL,
		client:     subscribe.NewClient(hubURL, 30*time.Second),
		dispatchCh: make(chan transport.Dispatch, 64),
		eventIDs:   make(chan string, 64),
		stopCh:     make(chan struct{}),
	}
}

// DispatchTopic returns the dispatch topic the Engine subscribes to for the
// given runner id. The server's session/agent dispatch must publish to this
// exact topic for the runner to receive it.
func DispatchTopic(runnerID string) bus.Topic {
	return bus.FormatTopic(bus.TopicRunnerDispatch, runnerID)
}

// Start opens the SSE subscription to the runner's dispatch topic and starts
// the event-reader, dispatch-worker, and presence-ticker goroutines.
func (e *Engine) Start(ctx context.Context) error {
	topic := DispatchTopic(e.runnerID)

	lid, _ := e.lastEventID.Load().(string)
	stream, err := e.client.Subscribe(e.secret, []string{string(topic)}, lid)
	if err != nil {
		return err
	}
	e.setStream(stream)

	// Presence: best-effort online signal on connect.
	if err := SetOnline(ctx, e.hubURL, e.runnerID, e.secret); err != nil {
		log.Printf("set-online failed (non-fatal): %v", err)
	}

	// Presence ticker (30 s heartbeat).
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.stopCh:
				return
			case <-ticker.C:
				if err := Heartbeat(ctx, e.hubURL, e.runnerID, e.secret); err != nil {
					log.Printf("heartbeat failed: %v", err)
				}
			}
		}
	}()

	// Dispatch worker: processes one dispatch at a time.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.dispatchWorker(ctx)
	}()

	// Event reader with auto-reconnect on stream drop.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.readerLoop(ctx)
	}()

	return nil
}

// Stop closes the SSE stream, cancels goroutines, and marks the runner offline.
func (e *Engine) Stop() {
	if e.closed.Swap(true) {
		return
	}
	if s := e.getStream(); s != nil {
		s.Close()
	}
	close(e.stopCh)
	e.wg.Wait()
	// Best-effort offline signal.
	_ = SetOffline(context.Background(), e.hubURL, e.runnerID, e.secret)
}

func (e *Engine) setStream(s *subscribe.SSEStream) {
	e.stream.Store(s)
}

func (e *Engine) getStream() *subscribe.SSEStream {
	v := e.stream.Load()
	if v == nil {
		return nil
	}
	s, ok := v.(*subscribe.SSEStream)
	if !ok {
		return nil
	}
	return s
}

// readerLoop reads events from the SSE stream. When the stream drops it
// attempts reconnection with jittered backoff and Last-Event-ID resume.
func (e *Engine) readerLoop(ctx context.Context) {
	for {
		if e.closed.Load() || ctx.Err() != nil {
			return
		}

		stream := e.getStream()
		if stream == nil {
			if !e.reconnect(ctx) {
				return
			}
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case ev, ok := <-stream.Events():
			if !ok {
				// Stream closed; reconnect on next iteration.
				e.setStream(nil)
				continue
			}
			e.handleEvent(ctx, ev)
		}
	}
}

// handleEvent deserializes a dispatch from an SSE event and enqueues it.
func (e *Engine) handleEvent(ctx context.Context, ev bus.Event) {
	var d transport.Dispatch
	if err := json.Unmarshal(ev.Message.Payload, &d); err != nil {
		log.Printf("error deserializing dispatch from event %s: %v", ev.ID, err)
		e.lastEventID.Store(ev.ID)
		return
	}

	select {
	case <-ctx.Done():
		return
	case <-e.stopCh:
		return
	case e.dispatchCh <- d:
		e.eventIDs <- ev.ID
	}

	e.lastEventID.Store(ev.ID)
}

// dispatchWorker pops dispatches one at a time from the buffered channel and
// processes them serially through the pull-run-report-ack lifecycle.
func (e *Engine) dispatchWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case d := <-e.dispatchCh:
			eventID := <-e.eventIDs
			if e.dispatchHook != nil {
				e.dispatchHook(d, eventID)
				continue
			}
			opts := processOpts{
				hubURL:     e.hubURL,
				catalogURL: e.catalogURL,
				secret:     e.secret,
				client:     e.client,
			}
			if err := processDispatch(ctx, d, eventID, opts); err != nil {
				log.Printf("dispatch processing error: %v", err)
			}
		}
	}
}

// reconnect creates a new SSE subscription after a connection drop, with
// exponential backoff and jitter. Returns true on success, false if the
// context was cancelled or the engine was stopped.
func (e *Engine) reconnect(ctx context.Context) bool {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if e.closed.Load() || ctx.Err() != nil {
			return false
		}

		// Jitter: up to 25 % of current backoff.
		jitter := time.Duration(rand.Int63n(int64(backoff) / 4))
		select {
		case <-ctx.Done():
			return false
		case <-e.stopCh:
			return false
		case <-time.After(backoff + jitter):
		}

		topic := bus.FormatTopic(bus.TopicRunnerDispatch, e.runnerID)
		lid, _ := e.lastEventID.Load().(string)

		stream, err := e.client.Subscribe(e.secret, []string{string(topic)}, lid)
		if err != nil {
			log.Printf("reconnect failed: %v (retrying in %v)", err, backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		e.setStream(stream)
		log.Printf("reconnected to SSE stream (last_event_id=%q)", lid)
		return true
	}
}
