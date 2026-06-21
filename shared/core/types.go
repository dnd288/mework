// Package core holds the canonical domain types shared across all mework
// components. These are stub definitions that downstream changes will fill in.
package core

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
	SandboxID string
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
}

// Hook is a lifecycle hook (before/after run, before/after agent step).
type Hook struct {
	Name   string
	Script string
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
