// Package core holds the canonical domain types shared across all mework
// components. These are stub definitions that downstream changes will fill in.
package core

import "time"

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
	ID          string
	Topic       Topic
	Payload     []byte
	ContentType string
}

// ResourceLimits constrains a sandboxed run.
type ResourceLimits struct {
	CPU    string `json:"cpu,omitempty"`    // e.g. "1.0", "500m"
	Memory string `json:"memory,omitempty"` // e.g. "512M", "2GiB"
	Disk   string `json:"disk,omitempty"`   // e.g. "10GiB"
}

// SandboxState describes the lifecycle state of a sandbox.
type SandboxState string

const (
	SandboxStateRunning   SandboxState = "running"
	SandboxStateStopped   SandboxState = "stopped"
	SandboxStateDestroyed SandboxState = "destroyed"
	SandboxStateCrashed   SandboxState = "crashed"
)

// RunSpec describes how to run an agent: which agent, which task, resource limits.
type RunSpec struct {
	AgentID        string            `json:"agent_id"`
	Task           string            `json:"task"`
	SandboxID      string            `json:"sandbox_id"`
	BackendName    string            `json:"backend_name,omitempty"`    // AI CLI name (e.g. "claude")
	BackendPath    string            `json:"backend_path,omitempty"`    // AI CLI binary path
	Image          string            `json:"image,omitempty"`           // container image for image-based drivers
	Timeout        time.Duration     `json:"timeout,omitempty"`         // wall-clock timeout (0 = no timeout)
	ResourceLimits *ResourceLimits   `json:"resource_limits,omitempty"` // CPU/memory/disk caps
	Env            map[string]string `json:"env,omitempty"`             // extra environment variables
}

// Result is the output of a completed agent run.
type Result struct {
	RunID    string `json:"run_id,omitempty"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Workspace is a synced working directory for an agent run.
type Workspace struct {
	ID   string `json:"id"`
	Path string `json:"path,omitempty"`
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

// SandboxCaps describes what a sandbox engine can do.
type SandboxCaps struct {
	MaxMemoryMB  int    `json:"max_memory_mb,omitempty"`
	MaxDiskMB    int    `json:"max_disk_mb,omitempty"`
	SupportsGPU  bool   `json:"supports_gpu"`
	SupportsNet  bool   `json:"supports_net"`
	IsIsolated   bool   `json:"is_isolated"`
	IsRemote     bool   `json:"is_remote"`     // true if sandbox runs on a remote service
	DriverName   string `json:"driver_name"`   // e.g. "local", "docker", "cloudflare"
	DefaultImage string `json:"default_image,omitempty"` // default image for container drivers
}
