// Package session manages user-agent sessions — the create→attach→close
// lifecycle, ownership enforcement, tenant-scoped listing, and idle reaping.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"mework/libs/server/bus"
	"mework/libs/server/auth"
	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// Config configures the session manager.
type Config struct {
	// IdleTimeout is the maximum time a session may be idle before being reaped.
	IdleTimeout time.Duration
	// ReapInterval controls how often the reaper checks for idle sessions.
	ReapInterval time.Duration
}

// DefaultConfig returns sensible defaults for session management.
func DefaultConfig() Config {
	return Config{
		IdleTimeout:  30 * time.Minute,
		ReapInterval: 5 * time.Minute,
	}
}

// sessionRecord is the internal bookkeeping for one session.
type sessionRecord struct {
	info         core.SessionInfo
	broker       bus.Broker
	sub          bus.Subscription
	lastActivity time.Time
	mu           sync.Mutex
}

// Manager implements ports.SessionManager with an in-memory session store,
// bus-backed live sessions, and a background reaper goroutine.
type Manager struct {
	mu       sync.RWMutex
	sessions map[core.SessionID]*sessionRecord
	broker   bus.Broker
	cfg      Config
	stopCh   chan struct{}
}

// NewManager creates a new session manager and starts the idle reaper.
func NewManager(broker bus.Broker, cfg Config) *Manager {
	m := &Manager{
		sessions: make(map[core.SessionID]*sessionRecord),
		broker:   broker,
		cfg:      cfg,
		stopCh:   make(chan struct{}),
	}
	go m.reapLoop()
	return m
}

// Stop stops the idle reaper goroutine. The manager can still be used
// after Stop, but idle sessions will no longer be reaped automatically.
func (m *Manager) Stop() {
	close(m.stopCh)
}

// Create creates a new tracked session. The returned SessionInfo carries the
// session's id, tenant, runner, agent, initial status (active), owner, and
// creation time.
func (m *Manager) Create(ctx context.Context, agentName, agentVersion, runnerID string, owner core.AccountID, tenant core.TenantID) (core.SessionInfo, error) {
	id, err := newSessionID()
	if err != nil {
		return core.SessionInfo{}, fmt.Errorf("generate session id: %w", err)
	}

	info := core.SessionInfo{
		ID:     id,
		Tenant: tenant,
		Runner: runnerID,
		Agent: core.Agent{
			ID:   agentName,
			Kind: agentVersion,
			Name: agentName,
		},
		Status:  core.SessionActive,
		Owner:   owner,
		Created: time.Now().UTC(),
	}

	// Subscribe to the session control topic so Attach can deliver events.
	controlTopic := bus.FormatTopic(bus.TopicSessionControl, string(id))
	sub, err := m.broker.Subscribe(ctx, bus.Identity(id), bus.Filter(controlTopic), "")
	if err != nil {
		return core.SessionInfo{}, fmt.Errorf("subscribe to control topic: %w", err)
	}

	m.mu.Lock()
	m.sessions[id] = &sessionRecord{
		info:         info,
		broker:       m.broker,
		sub:          sub,
		lastActivity: time.Now().UTC(),
	}
	m.mu.Unlock()

	return info, nil
}

// Get returns the current SessionInfo for the given session.
func (m *Manager) Get(_ context.Context, id core.SessionID) (core.SessionInfo, error) {
	m.mu.RLock()
	rec, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return core.SessionInfo{}, fmt.Errorf("session %q not found", id)
	}

	rec.mu.Lock()
	info := rec.info
	rec.mu.Unlock()

	return info, nil
}

// List returns all sessions scoped to the given tenant.
func (m *Manager) List(_ context.Context, tenant core.TenantID) ([]core.SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []core.SessionInfo
	for _, rec := range m.sessions {
		rec.mu.Lock()
		if rec.info.Tenant == tenant {
			result = append(result, rec.info)
		}
		rec.mu.Unlock()
	}
	return result, nil
}

// Attach returns the live wire endpoint for an existing session.
// Ownership is enforced: only the owning account (extracted from context via
// auth.GetAccountID) may attach. Returns an error if the session is closed
// or the caller is not the owner.
func (m *Manager) Attach(ctx context.Context, id core.SessionID) (ports.Session, error) {
	caller, _ := auth.GetAccountID(ctx)

	m.mu.RLock()
	rec, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.info.Status == core.SessionClosed {
		return nil, fmt.Errorf("session %q is closed", id)
	}

	// Enforce ownership.
	if caller != "" && rec.info.Owner != core.AccountID(caller) {
		return nil, fmt.Errorf("session %q is owned by %q, caller is %q", id, rec.info.Owner, caller)
	}

	rec.lastActivity = time.Now().UTC()
	rec.info.Status = core.SessionActive

	// Create a fresh subscription to the control topic so the caller
	// receives events from this point forward.
	controlTopic := bus.FormatTopic(bus.TopicSessionControl, string(id))
	sub, err := m.broker.Subscribe(ctx, bus.Identity(id), bus.Filter(controlTopic), "")
	if err != nil {
		return nil, fmt.Errorf("subscribe to control topic: %w", err)
	}

	return &liveSession{
		id:     id,
		broker: m.broker,
		sub:    sub,
	}, nil
}

// Close transitions a session to the closed (terminal) state and closes its
// bus subscription. Returns an error if the session does not exist.
func (m *Manager) Close(_ context.Context, id core.SessionID) error {
	m.mu.Lock()
	rec, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %q not found", id)
	}

	rec.mu.Lock()
	rec.info.Status = core.SessionClosed
	rec.sub.Close()
	rec.mu.Unlock()

	m.mu.Unlock()
	return nil
}

// reapLoop periodically checks for idle sessions and closes them.
func (m *Manager) reapLoop() {
	ticker := time.NewTicker(m.cfg.ReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.reap()
		}
	}
}

func (m *Manager) reap() {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rec := range m.sessions {
		rec.mu.Lock()
		if rec.info.Status == core.SessionClosed {
			rec.mu.Unlock()
			continue
		}
		if now.Sub(rec.lastActivity) > m.cfg.IdleTimeout {
			rec.info.Status = core.SessionClosed
			rec.sub.Close()
		}
		rec.mu.Unlock()
	}
}

// newSessionID generates a random hex session ID.
func newSessionID() (core.SessionID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return core.SessionID(hex.EncodeToString(b)), nil
}

// liveSession implements ports.Session backed by the message bus.
type liveSession struct {
	id     core.SessionID
	broker bus.Broker
	sub    bus.Subscription
}

func (s *liveSession) ID() string { return string(s.id) }

func (s *liveSession) Events() <-chan ports.Event {
	ch := make(chan ports.Event, 64)
	go func() {
		for ev := range s.sub.Events() {
			ch <- ports.Event{ID: ev.ID, Payload: ev.Message.Payload}
		}
		close(ch)
	}()
	return ch
}

func (s *liveSession) Push(ctx context.Context, payload []byte) error {
	topic := bus.FormatTopic(bus.TopicSessionControl, string(s.id))
	return s.broker.Publish(ctx, topic, bus.Message{Payload: payload})
}

func (s *liveSession) Close() error {
	return s.sub.Close()
}
