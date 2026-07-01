package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// manager is the concrete implementation of WorkspaceManager.
type manager struct {
	mu        sync.RWMutex
	sessions  map[string]*WorkspaceSession
	store     ports.ObjectStore
	published map[string]string // destName -> sourcePath (for shared-root publishing)
}

// NewWorkspaceManager creates a new WorkspaceManager backed by the given ObjectStore.
func NewWorkspaceManager(store ports.ObjectStore) WorkspaceManager {
	return &manager{
		sessions:  make(map[string]*WorkspaceSession),
		store:     store,
		published: make(map[string]string),
	}
}

// Attach creates a new workspace session bound to the given spec.
func (m *manager) Attach(ctx context.Context, spec core.WorkspaceSpec) (*WorkspaceSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	session := &WorkspaceSession{
		ID:        sessionID,
		Spec:      spec,
		MountPath: spec.MountPath,
		Status: WorkspaceStatus{
			SessionID: sessionID,
			SyncMode:  spec.Sync,
		},
	}

	m.sessions[sessionID] = session
	return session, nil
}

// Get retrieves an active workspace session by ID.
func (m *manager) Get(ctx context.Context, sessionID string) (*WorkspaceSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("workspace session not found: %s", sessionID)
	}
	return session, nil
}

// Detach performs a final flush of pending writes to the object store, then
// unmounts and removes the session. Detach always flushes regardless of sync mode.
func (m *manager) Detach(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("workspace session not found: %s", sessionID)
	}

	// Final flush regardless of sync mode.
	if _, err := m.syncLocked(ctx, session); err != nil {
		return fmt.Errorf("final sync on detach failed: %w", err)
	}

	delete(m.sessions, sessionID)
	return nil
}

// Sync forces a sync of the workspace to the object store.
// Pushes local changes to the remote prefix and pulls any remote changes down.
func (m *manager) Sync(ctx context.Context, sessionID string) (*core.SyncResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("workspace session not found: %s", sessionID)
	}

	return m.syncLocked(ctx, session)
}

// syncLocked performs the actual sync against the object store.
// Must be called with m.mu held.
func (m *manager) syncLocked(ctx context.Context, session *WorkspaceSession) (*core.SyncResult, error) {
	result := &core.SyncResult{}

	prefix := session.Spec.RemotePrefix
	if prefix == "" {
		prefix = "sessions/" + session.ID
	}

	// List remote objects under the session prefix.
	objects, err := m.store.List(ctx, prefix+"/")
	if err != nil {
		return result, fmt.Errorf("sync list failed: %w", err)
	}

	// Count pulled objects (actual data transfer is a stub until c0008 lands).
	for range objects {
		result.Pulled++
	}

	session.Status.LastSyncTime = time.Now()
	session.Status.PushedCount += result.Pushed
	session.Status.PulledCount += result.Pulled
	session.Status.FailedCount += result.Failed

	return result, nil
}

// Status returns the current observable state of the workspace session.
func (m *manager) Status(ctx context.Context, sessionID string) (*WorkspaceStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("workspace session not found: %s", sessionID)
	}

	status := session.Status
	return &status, nil
}

// MountSharedRoot mounts a read-only union of all published folders at rootPath.
// The mount is tracked in the session so subsequent reads through WorkspaceFS
// can access the shared root.
func (m *manager) MountSharedRoot(ctx context.Context, sessionID string, rootPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("workspace session not found: %s", sessionID)
	}

	// Add all published destinations as shared roots for this session.
	for dest, src := range m.published {
		publishedPath := rootPath + "/" + dest
		session.Spec.SharedRoots = append(session.Spec.SharedRoots, src)
		_ = publishedPath // reserved for future bind-mount logic
	}

	return nil
}

// Publish promotes a sub-path from the workspace into the shared namespace
// under the given destination name. Only paths within the workspace root are
// allowed; push outside the allowed scope is denied (enforced at the call site
// through grant verification).
func (m *manager) Publish(ctx context.Context, sessionID string, sourcePath string, destName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("workspace session not found: %s", sessionID)
	}

	if destName == "" {
		return fmt.Errorf("publish destination name cannot be empty")
	}

	// Track the published path.
	m.published[destName] = sourcePath
	return nil
}

// Bootstrap materializes the workspace base from the BaseSpec (git clone,
// archive unpack, or store copy) then runs init hooks. A failing init hook
// aborts the run — the caller should report failure and tear down the sandbox.
func (m *manager) Bootstrap(ctx context.Context, sessionID string) (*core.HookResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("workspace session not found: %s", sessionID)
	}

	base := session.Spec.Base
	if base != nil {
		switch base.Kind {
		case core.BaseKindGit:
			// Stub: real git clone logic lands with sandbox runtime (c0006).
		case core.BaseKindArchive:
			// Stub: real archive unpack logic lands with c0008.
		case core.BaseKindStore:
			// Stub: real store copy logic lands with c0008.
		}
	}

	// Run init hooks after base materialization.
	return m.runHooksLocked(ctx, session, core.HookStageInit)
}

// RunHooks drives the hooks for the given lifecycle stage. The stage must be
// one of: pre_run, post_run, pre_sync, post_sync. (init is run by Bootstrap.)
func (m *manager) RunHooks(ctx context.Context, sessionID string, stage core.HookStage) (*core.HookResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("workspace session not found: %s", sessionID)
	}

	return m.runHooksLocked(ctx, session, stage)
}

// runHooksLocked executes all hooks matching the given stage.
// Must be called with m.mu held. Hook scripts are conceptually delivered over
// stdin (not argv) to preserve the injection-safe invariant — the sandbox
// driver enforces this at runtime.
func (m *manager) runHooksLocked(ctx context.Context, session *WorkspaceSession, stage core.HookStage) (*core.HookResult, error) {
	result := &core.HookResult{Stage: stage}

	for _, hook := range session.Spec.Hooks {
		if hook.Stage != stage {
			continue
		}

		// Stub: hook execution is delegated to the sandbox driver (c0006).
		// In production, the sandbox receives the script on stdin and runs it
		// within the grant scope.
		_ = hook.Script

		result.Output += fmt.Sprintf("[stub] hook %q at stage %s would run\n", hook.Name, stage)
	}

	return result, nil
}
