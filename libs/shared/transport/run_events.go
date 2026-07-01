// Package transport holds the wire contracts — the SSE event schema, API DTOs,
// and the runnerâ†"sandbox protocol. These are shared across all mework
// components so each contract has one source of truth.
package transport

import "context"

// RunStatus is the lifecycle status of a run.
type RunStatus string

const (
	StatusQueued   RunStatus = "queued"
	StatusRunning  RunStatus = "running"
	StatusDone     RunStatus = "done"
	StatusFailed   RunStatus = "failed"
	StatusCanceled RunStatus = "canceled"
)

// RunEvent is an upstream event the runner/agent emits about a run.
type RunEvent struct {
	Kind string // "progress" | "log" | "output" | "status"
	Data []byte
}

// Subscription delivers events for a subscribed run.
type Subscription interface {
	// Events returns a channel that delivers RunEvents for the subscribed run.
	Events() <-chan RunEvent
	// Close terminates the subscription.
	Close() error
}

// RunEvents provides live run telemetry and run-level control.
// The runner/agent emits upstream events; clients subscribe to a run's live
// stream; run status is queryable; and a run can be cancelled.
type RunEvents interface {
	// Emit publishes an upstream event from the runner/agent for the given run.
	Emit(ctx context.Context, runID string, ev RunEvent) error

	// Subscribe returns a live subscription to a run's events.
	// Late subscribers first receive a bounded recent tail and are then spliced
	// into the live stream with no gap or duplication at the boundary.
	Subscribe(ctx context.Context, runID string) (Subscription, error)

	// Status returns the run's current status, queryable at any time.
	Status(ctx context.Context, runID string) (RunStatus, error)

	// Cancel requests cancellation of a run. When force is false, the run is
	// asked to stop gracefully; when force is true, termination is forced.
	// Cancel is idempotent and terminal: re-issuing cancel on an already
	// canceled run is a no-op, and a canceled run cannot resume.
	Cancel(ctx context.Context, runID string, force bool) error
}
