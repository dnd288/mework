// Package ports holds the pluggable interfaces — the ports that define the
// boundaries between mework components. Every swappable backend implements
// a port defined here. Consumers import shared/ports, never a concrete driver.
package ports

import (
	"context"
	"errors"
	"io"
	"mework/shared/core"
)

// Sentinel errors for the RunnerSelector and SecretInjector ports.
var (
	// ErrNoEligibleRunner is returned by RunnerSelector.Select when no runner
	// is eligible for the dispatch.
	ErrNoEligibleRunner = errors.New("no eligible runner found for dispatch")

	// ErrSecretRefused is returned by SecretInjector.Inject when a secret's
	// source is not within the dispatch's grant scope.
	ErrSecretRefused = errors.New("secret source not in grant scope")
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
	// ID returns the sandbox identifier, set at Start time by the driver.
	ID() string
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

// SelectionCriteria defines what to select a runner for.
type SelectionCriteria struct {
	// AgentRef is the agent that the dispatch will run.
	AgentRef string
	// SessionID, if set, requests session-affinity routing.
	SessionID string
}

// RunnerSelector selects a target runner for a dispatch, load-balancing
// across eligible online runners and honouring session affinity.
type RunnerSelector interface {
	// Select returns the ID of the best eligible runner for the given
	// tenant and selection criteria. Returns ErrNoEligibleRunner when
	// no runner is eligible.
	Select(ctx context.Context, tenant string, criteria SelectionCriteria) (string, error)
}

// SecretRef identifies a single secret to inject into a sandbox.
type SecretRef struct {
	// Name is the logical name of the secret (e.g. "API_KEY").
	Name string
	// Source is the grant source that scopes this secret (e.g. "github" or "openai").
	Source string
}

// SecretInjector delivers grant-scoped secrets into a provisioned sandbox
// out-of-band (env / file), never via argv or logs.
type SecretInjector interface {
	// Inject materialises each secret as a per-sandbox file with 0400
	// permissions and exposes it via an env var whose name is grant-scoped.
	// Each secret's Source must be in the provided sources list; otherwise
	// ErrSecretRefused is returned and the sandbox is aborted.
	Inject(ctx context.Context, sandboxID string, sources []string, secrets []SecretRef) error
}
