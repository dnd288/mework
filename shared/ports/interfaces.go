// Package ports holds the pluggable interfaces — the ports that define the
// boundaries between mework components. Every swappable backend implements
// a port defined here. Consumers import shared/ports, never a concrete driver.
package ports

import (
	"context"
	"io"
	"time"

	"mework/shared/core"
)

// SandboxDriver manages the lifecycle of sandbox environments.
type SandboxDriver interface {
	// Caps returns the capabilities of this driver.
	Caps() core.SandboxCaps
	// Start creates and starts a new sandbox.
	Start(ctx context.Context, spec core.RunSpec) (Sandbox, error)
	// Stop stops and destroys a sandbox.
	Stop(ctx context.Context, sandboxID string) error
	// Destroy forcibly removes a sandbox.
	Destroy(ctx context.Context, sandboxID string) error
}

// Sandbox is a running sandbox environment that can execute commands.
type Sandbox interface {
	// Exec runs a command inside the sandbox and connects stdio.
	Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
	// Mount syncs a workspace into the sandbox.
	Mount(ctx context.Context, workspace core.Workspace, targetPath string) error
	// Signals sends a signal to the running process.
	Signals(ctx context.Context, sig string) error
}

// ObjectStore is a generic key-value blob store interface.
type ObjectStore interface {
	Put(ctx context.Context, ref core.ObjectRef, reader io.Reader) error
	Get(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error)
	Delete(ctx context.Context, ref core.ObjectRef) error
	List(ctx context.Context, prefix string) ([]core.ObjectInfo, error)
	PresignGetURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, error)
	PresignPutURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, error)
}

// AgentBackend detects and runs AI coding agents (CLI tools).
type AgentBackend interface {
	// Detect checks if this backend is available on the system.
	Detect(ctx context.Context) bool
	// Run executes the agent with the given prompt and workspace.
	Run(ctx context.Context, spec core.RunSpec, workspace core.Workspace, stdout, stderr io.Writer) (core.Result, error)
}

// Broker is a publish/subscribe message bus interface.
type Broker interface {
	Publish(ctx context.Context, topic core.Topic, msg core.Message) error
	Subscribe(ctx context.Context, topic core.Topic, handler func(core.Message) error) error
	Queue(ctx context.Context, topic core.Topic, msg core.Message) error
}

// ProviderAdapter is the interface for provider-specific adapters that
// translate between mework's generic model and the provider's REST API.
type ProviderAdapter interface {
	Validate(ctx context.Context, settings map[string]string) error
	CreateComment(ctx context.Context, ticketID, body string) error
	// Add more adapter methods as needed by downstream consumers.
}

// Notifier sends outbound notifications on platform events.
type Notifier interface {
	Notify(ctx context.Context, tenant core.TenantID, event core.NotifyEvent) error
	DeliveryStatus(ctx context.Context, runID string) ([]DeliveryResult, error)
}

// DeliveryResult records the outcome of a notification delivery attempt.
type DeliveryResult struct {
	ID           string `json:"id"`
	RunID        string `json:"run_id"`
	EventKind    string `json:"event_kind"`
	Status       string `json:"status"`
	AttemptCount int    `json:"attempt_count"`
	LastStatus   int    `json:"last_status"`
	LastError    string `json:"last_error"`
}

// ArtifactStore persists and serves run outputs/artifacts.
type ArtifactStore interface {
	Put(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef, content []byte) error
	Get(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef) ([]byte, error)
	List(ctx context.Context, tenant core.TenantID, runID string) ([]core.ArtifactInfo, error)
	PresignGetURL(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef, ttl time.Duration) (string, error)
	PresignPutURL(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef, ttl time.Duration) (string, error)
}

// Scheduler manages time-based dispatches of agents. Each schedule produces
// dispatch messages through the catalog/orchestrator when its fire time arrives.
type Scheduler interface {
	// Schedule creates a new schedule from the given spec and returns its ID.
	// The schedule is created in the active state and begins firing immediately.
	Schedule(ctx context.Context, tenantID string, spec core.ScheduleSpec) (string, error)

	// Pause suppresses fires for the given schedule without discarding it.
	// A paused schedule never fires; Pause is idempotent.
	Pause(ctx context.Context, tenantID, scheduleID string) error

	// Resume re-arms a paused schedule so it becomes eligible to fire again.
	// Resume is idempotent.
	Resume(ctx context.Context, tenantID, scheduleID string) error

	// Cancel permanently removes a schedule. A canceled schedule never fires.
	// Cancel is idempotent; canceling a removed schedule returns no error.
	Cancel(ctx context.Context, tenantID, scheduleID string) error

	// List returns all non-canceled schedule IDs for the given tenant.
	// Results are scoped to the tenant; cross-tenant visibility is forbidden.
	List(ctx context.Context, tenantID string) ([]string, error)

	// Get returns the schedule spec and state for a given schedule ID.
	Get(ctx context.Context, tenantID, scheduleID string) (*core.ScheduleSpec, core.ScheduleState, error)
}
