// Package runtime manages the lifecycle of a sandbox environment: start, stop,
// hooks, workspace mount, and teardown. It delegates to the active engine
// via the sandbox driver interface from shared/ports.
package runtime

import (
	"context"
	"fmt"
	"sync"

	"mework/sandbox/engine/cloudflare"
	"mework/sandbox/engine/custom"
	"mework/sandbox/engine/docker"
	"mework/sandbox/engine/local"
	"mework/shared/core"
	"mework/shared/ports"
)

// Manager manages a sandbox environment through its lifecycle.
type Manager struct {
	mu     sync.Mutex
	driver ports.SandboxDriver
	running map[string]ports.Sandbox // active sandboxes keyed by ID
}

// NewManager creates a new sandbox Manager with the given engine.
func NewManager(driver ports.SandboxDriver) *Manager {
	return &Manager{
		driver:  driver,
		running: make(map[string]ports.Sandbox),
	}
}

// NewManagerFor creates a Manager for the named engine ("local", "docker",
// "cloudflare", "custom"). An empty or unknown name defaults to "local".
func NewManagerFor(engine string) *Manager {
	switch engine {
	case "docker":
		return NewManager(docker.New())
	case "cloudflare":
		return NewManager(cloudflare.New())
	case "custom":
		if d, err := custom.New(); err == nil {
			return NewManager(d)
		}
		// Fall back to local if custom engine is unavailable.
		return NewManager(local.New())
	default:
		return NewManager(local.New())
	}
}

// Caps returns the capabilities of the managed driver.
func (m *Manager) Caps() core.SandboxCaps {
	return m.driver.Caps()
}

// Start creates and starts a new sandbox for the given spec.
func (m *Manager) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Enforce one agent per sandbox: check for duplicate ID.
	if _, exists := m.running[spec.SandboxID]; exists {
		return nil, fmt.Errorf("sandbox %q already exists", spec.SandboxID)
	}

	s, err := m.driver.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	m.running[s.ID()] = s
	return s, nil
}

// Stop stops a running sandbox gracefully.
func (m *Manager) Stop(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.running[sandboxID]
	if !exists {
		return fmt.Errorf("sandbox %q not found", sandboxID)
	}

	if err := m.driver.Stop(ctx, sandboxID); err != nil {
		return err
	}

	delete(m.running, sandboxID)
	return nil
}

// Destroy forcibly removes a sandbox and its resources.
func (m *Manager) Destroy(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.driver.Destroy(ctx, sandboxID); err != nil {
		return err
	}

	delete(m.running, sandboxID)
	return nil
}

// DestroyAll stops and removes all active sandboxes.
func (m *Manager) DestroyAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id := range m.running {
		if err := m.driver.Destroy(ctx, id); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
		delete(m.running, id)
	}
	return firstErr
}

// Hooks returns the lifecycle hooks supported by this sandbox.
func (m *Manager) Hooks() []string {
	return nil
}

// WorkspaceMount returns the path where the workspace is mounted.
func (m *Manager) WorkspaceMount() string {
	return ""
}

// Teardown cleans up all sandbox environments.
func (m *Manager) Teardown(ctx context.Context) error {
	return m.DestroyAll(ctx)
}

// ActiveCount returns the number of currently running sandboxes.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running)
}
