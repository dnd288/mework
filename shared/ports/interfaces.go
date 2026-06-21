// Package ports holds the pluggable interfaces — the ports that define the
// boundaries between mework components. Every swappable backend implements
// a port defined here. Consumers import shared/ports, never a concrete driver.
package ports

import (
	"context"
	"io"
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

// Notifier sends notifications through various channels.
type Notifier interface {
	Notify(ctx context.Context, channel string, title, body string) error
}

// Event is a single event delivered on a session control channel.
type Event struct {
	ID      string
	Payload []byte
}

// Session is the live wire endpoint of a session — a control channel plus
// the ability to push messages to the sandbox. It is the value Attach returns.
type Session interface {
	// ID returns the session's unique identifier.
	ID() core.SessionID
	// Events returns a channel that delivers control events for this session.
	Events() <-chan Event
	// Push sends a message payload to the sandbox.
	Push(ctx context.Context, payload []byte) error
	// Close terminates the live endpoint.
	Close() error
}

// SessionManager owns the session lifecycle (separate from the bus Session
// primitive, which is the live wire endpoint Attach returns).
type SessionManager interface {
	// Create creates a new tracked session from the given agent/runner params.
	Create(ctx context.Context, agentName, agentVersion, runnerID string, owner core.AccountID, tenant core.TenantID) (core.SessionInfo, error)
	// Get returns the current SessionInfo for a session.
	Get(ctx context.Context, id core.SessionID) (core.SessionInfo, error)
	// List returns all sessions scoped to a tenant.
	List(ctx context.Context, tenant core.TenantID) ([]core.SessionInfo, error)
	// Attach returns the live wire endpoint for an existing session.
	Attach(ctx context.Context, id core.SessionID) (Session, error)
	// Close terminates a session and destroys its sandbox.
	Close(ctx context.Context, id core.SessionID) error
}
