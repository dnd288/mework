// Package core holds the canonical domain types shared across all mework
// components. These are stub definitions that downstream changes will fill in.
package core

import (
	"fmt"
	"time"
)

// Agent represents an AI coding agent that can be run in a sandbox.

// Agent represents an AI coding agent that can be run in a sandbox.
type Agent struct {
	ID   string
	Kind string
	Name string
}

// Run is a single execution of an agent on a task.
type Run struct {
	ID      string
	AgentID string
	Spec    RunSpec
}

// Session represents a long-lived interaction between a user and an agent.
type Session struct {
	ID      string
	AgentID string
	UserID  string
}

// Grant is a signed permission allowing an agent to access a resource.
type Grant struct {
	ID       string
	Resource string
	Action   string
}

// Topic is a message-bus topic name.
type Topic struct {
	Name string
}

// Message is an event published on the message bus.
type Message struct {
	ID        string
	Topic     Topic
	Payload   []byte
	ContentType string
}

// RunSpec describes how to run an agent: which agent, which task, resource limits.
type RunSpec struct {
	AgentID   string
	Task      string
	SandboxID   string
	BackendPath string
	BackendName string
	Image       string
	Env         map[string]string
	ResourceLimits *ResourceLimits
	Timeout      time.Duration
}

// Result is the output of a completed agent run.
type Result struct {
	RunID    string
	ExitCode int
	Output   string
	Error    string
}

// Workspace is a synced working directory for an agent run.
type Workspace struct {
	ID   string
	Path string
}

// ObjectRef identifies an object in an object store (bucket + key).
type ObjectRef struct {
	Bucket string
	Key    string
}

// ObjectInfo is metadata about a stored object.
type ObjectInfo struct {
	Ref       ObjectRef
	Size      int64
	ETag      string
	LastModified time.Time
}

// Hook is a lifecycle hook (before/after run, before/after agent step).
type Hook struct {
	Name   string
	Script string
	Stage  HookStage
}

// TenantID uniquely identifies a tenant in the system.
type TenantID string

// NotifyEvent is an event that triggers an outbound notification.
type NotifyEvent struct {
	Kind   string // "run.done" | "run.failed"
	RunID  string
	Target string // outbound webhook URL
}

// ArtifactRef identifies a run artifact.
type ArtifactRef struct {
	RunID    string
	Name     string
	Checksum string
}

// ArtifactInfo is metadata about a stored artifact.
type ArtifactInfo struct {
	Ref       ArtifactRef
	Size      int64
	CreatedAt string
}

// SandboxCaps describes what a sandbox engine can do.
type SandboxCaps struct {
	MaxMemoryMB    int
	MaxDiskMB      int
	SupportsGPU    bool
	SupportsNet    bool
	IsIsolated     bool
	IsRemote       bool
	DriverName     string
}

// ScheduleKind enumerates the kinds of schedules.
type ScheduleKind string

const (
	ScheduleCron     ScheduleKind = "cron"
	ScheduleInterval ScheduleKind = "interval"
	ScheduleAt       ScheduleKind = "at"
)

// MissedPolicy governs what happens when a fire time elapses while the
// target runner is offline.
type MissedPolicy string

const (
	MissedSkip    MissedPolicy = "skip"
	MissedCatchUp MissedPolicy = "catch_up"
)

// ScheduleState is the lifecycle state of a schedule.
type ScheduleState string

const (
	ScheduleActive  ScheduleState = "active"
	SchedulePaused  ScheduleState = "paused"
	ScheduleCanceled ScheduleState = "canceled"
)

// ScheduleSpec describes a single schedule — what to run, when, and how to
// handle missed fires. Exactly one of the time fields applies depending on Kind.
type ScheduleSpec struct {
	// Kind selects the schedule kind: cron, interval, or at.
	Kind ScheduleKind `json:"kind"`
	// Cron is a standard 5-field cron expression. Applicable when Kind=cron.
	Cron string `json:"cron,omitempty"`
	// Every is a duration string (e.g. "1h", "30m"). Applicable when Kind=interval.
	Every string `json:"every,omitempty"`
	// At is an RFC3339 instant. Applicable when Kind=at.
	At string `json:"at,omitempty"`
	// TZ is an IANA timezone name (e.g. "Asia/Ho_Chi_Minh"). Used for cron evaluation.
	TZ string `json:"tz,omitempty"`
	// Agent is the name of the agent (and optional version) to dispatch.
	Agent string `json:"agent"`
	// Target is the runner ID to dispatch to.
	Target string `json:"target"`
	// Grant is an optional JSON-encoded grant to carry on the dispatch.
	Grant []byte `json:"grant,omitempty"`
	// Missed is the missed-fire policy when the runner is offline.
	Missed MissedPolicy `json:"missed,omitempty"`
}

// SessionID uniquely identifies a session.
type SessionID string

// AccountID uniquely identifies a user account.
type AccountID string

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	SessionActive SessionStatus = "active"
	SessionIdle   SessionStatus = "idle"
	SessionClosed SessionStatus = "closed"
)

// SessionInfo is the management view of a live agent association.
type SessionInfo struct {
	ID      SessionID
	Tenant  TenantID
	Runner  string
	Agent   Agent
	Status  SessionStatus
	Owner   AccountID
	Created time.Time
}

// ResourceLimits describes resource constraints for a sandbox.
type ResourceLimits struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
}

// SandboxState describes the lifecycle state of a sandbox.
type SandboxState string

const (
	SandboxStateRunning   SandboxState = "running"
	SandboxStateStopped   SandboxState = "stopped"
	SandboxStateDestroyed SandboxState = "destroyed"
	SandboxStateCrashed   SandboxState = "crashed"
)

// ObjectDeleted is a sentinel error returned when an object is not found.

// WorkspaceMode controls read-write vs read-only workspace mounts.
type WorkspaceMode string

const (
	WorkspaceModeRW WorkspaceMode = "rw"
	WorkspaceModeRO WorkspaceMode = "ro"
)

// SyncMode controls when workspace changes are pushed to the remote object store.
type SyncMode string

const (
	SyncModeContinuous SyncMode = "continuous"
	SyncModeOnFlush    SyncMode = "on_flush"
	SyncModeManual     SyncMode = "manual"
)

// BaseKind is the type of base source for a workspace.
type BaseKind string

const (
	BaseKindGit     BaseKind = "git"
	BaseKindArchive BaseKind = "archive"
	BaseKindStore   BaseKind = "store"
)

// BaseSpec describes how to materialize the workspace base before the agent runs.
type BaseSpec struct {
	Kind BaseKind
	Ref  string
	Rev  string
}

// HookStage identifies a point in the workspace lifecycle where hooks run.
type HookStage string

const (
	HookStageInit     HookStage = "init"
	HookStagePreRun   HookStage = "pre_run"
	HookStagePostRun  HookStage = "post_run"
	HookStagePreSync  HookStage = "pre_sync"
	HookStagePostSync HookStage = "post_sync"
)

// HookResult captures the outcome of a lifecycle hook execution.
type HookResult struct {
	Stage    HookStage
	ExitCode int
	Output   string
	Error    string
}

// SyncResult captures the outcome of a sync operation.
type SyncResult struct {
	Pushed int
	Pulled int
	Failed int
}

// WorkspaceSpec describes how to create and manage a workspace for a session.
type WorkspaceSpec struct {
	MountPath    string
	RemotePrefix string
	Mode         WorkspaceMode
	Sync         SyncMode
	SharedRoots  []string
	Base         *BaseSpec
	Hooks        []Hook
}



// ObjectDeleted is a sentinel error returned when an object is not found.
var ObjectDeleted = fmt.Errorf("object not found")

