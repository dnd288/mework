package bus

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mework/server/middleware"
)

// SSEHandler handles Server-Sent Events subscriptions over HTTP.
type SSEHandler struct {
	broker            Broker
	heartbeatInterval time.Duration
	bufferSize        int
}

// SSEOption configures an SSEHandler.
type SSEOption func(*SSEHandler)

// WithHeartbeatInterval sets the interval at which heartbeat comment lines are
// written to the SSE stream to keep proxy/load-balancer connections alive.
func WithHeartbeatInterval(d time.Duration) SSEOption {
	return func(h *SSEHandler) {
		h.heartbeatInterval = d
	}
}

// NewSSEHandler returns a new SSEHandler backed by the given broker.
func NewSSEHandler(broker Broker, opts ...SSEOption) *SSEHandler {
	h := &SSEHandler{
		broker:            broker,
		heartbeatInterval: 30 * time.Second,
		bufferSize:        64,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Subscribe handles GET requests for an SSE subscription. It reads the
// "topics" (comma-separated) and optional "last_event_id" query parameters,
// subscribes to each topic via the broker, and writes SSE frames to the
// response.
func (h *SSEHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	topicsStr := r.URL.Query().Get("topics")
	if topicsStr == "" {
		http.Error(w, "missing topics query parameter", http.StatusBadRequest)
		return
	}
	rawTopics := strings.Split(topicsStr, ",")

	lastEventID := r.URL.Query().Get("last_event_id")
	if lastEventID == "" {
		lastEventID = r.Header.Get("Last-Event-ID")
	}

	runtimeID, ok := middleware.GetRuntimeID(r.Context())
	if !ok {
		runtimeID = "unknown"
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send headers immediately so the client receives the 200 response
	// before we enter the write loop. This is essential for SSE: the client
	// needs to see the response before the server starts pushing events.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// mergedEvents carries SSE frames from all subscription goroutines to the
	// write loop. It is bounded so that a slow reader does not block publishing.
	mergedEvents := make(chan sseFrame, h.bufferSize)

	var wg sync.WaitGroup
	for _, raw := range rawTopics {
		topic := strings.TrimSpace(raw)
		if topic == "" {
			continue
		}
		sub, err := h.broker.Subscribe(ctx, Identity(runtimeID), Filter(topic), lastEventID)
		if err != nil {
			http.Error(w, fmt.Sprintf("subscribe error: %v", err), http.StatusInternalServerError)
			return
		}
		wg.Add(1)
		go func(sub Subscription) {
			defer wg.Done()
			defer sub.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-sub.Events():
					if !ok {
						return
					}
					f := sseFrame{
						id:    ev.ID,
						event: string(ev.Topic),
						data:  string(ev.Message.Payload),
					}
					// Non-blocking send with drop-oldest when the merged
					// channel is full, ensuring a slow subscriber does not
					// block the bus (bounded backpressure).
					select {
					case mergedEvents <- f:
					default:
						// Drop one oldest event to make room.
						select {
						case <-mergedEvents:
						default:
						}
						select {
						case mergedEvents <- f:
						default:
							// Still full; drop this event.
						}
					}
				}
			}
		}(sub)
	}

	// Heartbeat ticker keeps proxy connections alive.
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()

	// Close mergedEvents when all subscription goroutines finish.
	go func() {
		wg.Wait()
		close(mergedEvents)
	}()

	// Write loop: drain mergedEvents and heartbeat ticker until the client
	// disconnects or the context is cancelled.
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case f, ok := <-mergedEvents:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", f.id, f.event, f.data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// sseFrame is an internal type for passing event data between goroutines.
type sseFrame struct {
	id    string
	event string
	data  string
}
