// Package storage defines the object-store interface for server-side blob
// storage. It wraps the ObjectStore port from shared/ports. It also defines
// the WorkspaceManager for lifecycle management of session workspaces.
//
// This is a stub — the full implementation lands in a downstream change.
package storage

import (
	"context"
	"io"
	"time"

	"mework/libs/shared/core"
)

// Store is the server's object storage interface.
type Store interface {
	Put(ctx context.Context, ref core.ObjectRef, reader io.Reader) error
	Get(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error)
	Delete(ctx context.Context, ref core.ObjectRef) error
	List(ctx context.Context, prefix string) ([]core.ObjectInfo, error)
}

// WorkspaceStatus reports the observable state of a workspace session.
type WorkspaceStatus struct {
	SessionID     string
	LastSyncTime  time.Time
	PushedCount   int
	PulledCount   int
	FailedCount   int
	PendingWrites int
	SyncMode      core.SyncMode
}

// WorkspaceSession represents a live workspace mount bound to an object-store prefix.
type WorkspaceSession struct {
	ID        string
	Spec      core.WorkspaceSpec
	MountPath string
	Status    WorkspaceStatus
}

// WorkspaceManager manages the lifecycle of object-store-backed workspaces.
//
// Attach binds a session-scoped folder to a remote prefix and mounts it
// read-write at the spec's mount path. Detach performs a final flush then
// unmounts. Sync pushes/pulls against the ObjectStore. Status reports
// observable sync state. MountSharedRoot mounts a read-only union of
// published folders. Publish promotes the grant-allowed sub-path into
// the shared namespace. Bootstrap materializes the BaseSpec and runs
// init hooks. RunHooks drives the given lifecycle HookStage.
type WorkspaceManager interface {
	Attach(ctx context.Context, spec core.WorkspaceSpec) (*WorkspaceSession, error)
	Get(ctx context.Context, sessionID string) (*WorkspaceSession, error)
	Detach(ctx context.Context, sessionID string) error
	Sync(ctx context.Context, sessionID string) (*core.SyncResult, error)
	Status(ctx context.Context, sessionID string) (*WorkspaceStatus, error)
	MountSharedRoot(ctx context.Context, sessionID string, rootPath string) error
	Publish(ctx context.Context, sessionID string, sourcePath string, destName string) error
	Bootstrap(ctx context.Context, sessionID string) (*core.HookResult, error)
	RunHooks(ctx context.Context, sessionID string, stage core.HookStage) (*core.HookResult, error)
}

// Register registers a storage driver constructor.
func Register(name string, fn func(cfg Config) (Store, error)) {
	registry[name] = fn
}

var registry = make(map[string]func(Config) (Store, error))

