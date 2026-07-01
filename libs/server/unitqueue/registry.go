// Package unitqueue provides a name-to-session routing registry that lets
// agents expose a human-friendly "unit queue" name. Once an agent registers its
// name, any caller can send messages to that name and they are routed to the
// agent's current session — no opaque session ID needed.
//
// Concept: a unit queue is an agent that is online and addressable by name.
//   - An agent registers with Register() when it comes online, binding its
//     name to a session.
//   - Callers send messages via the name; the registry resolves name→session
//     and publishes to session.<id>.input.
//   - Responses flow back on session.<id>.control (existing SSE stream).
package unitqueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("unit queue: name not found")
	ErrAlreadyExists = errors.New("unit queue: name already registered")
)

// Registration binds a human-friendly agent name to a running session.
type Registration struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	RunnerID  string `json:"runner_id"`
	Status    string `json:"status"` // "online"
	Created   string `json:"created"`
}

// Registry maps agent names to active session+runner bindings.
type Registry interface {
	// Register binds name to sessionID+runnerID. Returns ErrAlreadyExists if
	// the name is already registered (call Unregister first).
	Register(ctx context.Context, name, sessionID, runnerID string) error

	// Unregister removes a name binding. Returns ErrNotFound if not registered.
	Unregister(ctx context.Context, name string) error

	// Lookup returns the Registration for name. Returns ErrNotFound if not
	// registered.
	Lookup(ctx context.Context, name string) (Registration, error)

	// List returns all active registrations.
	List(ctx context.Context) ([]Registration, error)
}

// NewMemoryRegistry returns an in-memory Registry (no persistence, lost on
// restart). Suitable for dev and testing; swap for a Postgres-backed registry
// in production.
func NewMemoryRegistry() Registry {
	return &memoryRegistry{
		entries: make(map[string]Registration),
	}
}

type memoryRegistry struct {
	mu      sync.RWMutex
	entries map[string]Registration
}

func (r *memoryRegistry) Register(_ context.Context, name, sessionID, runnerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[name]; exists {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}

	r.entries[name] = Registration{
		Name:      name,
		SessionID: sessionID,
		RunnerID:  runnerID,
		Status:    "online",
		Created:   time.Now().UTC().Format(time.RFC3339),
	}
	return nil
}

func (r *memoryRegistry) Unregister(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[name]; !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(r.entries, name)
	return nil
}

func (r *memoryRegistry) Lookup(_ context.Context, name string) (Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, exists := r.entries[name]
	if !exists {
		return Registration{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return reg, nil
}

func (r *memoryRegistry) List(_ context.Context) ([]Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Registration, 0, len(r.entries))
	for _, reg := range r.entries {
		result = append(result, reg)
	}
	return result, nil
}
