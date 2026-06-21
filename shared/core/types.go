// Package core holds the canonical domain types shared across all mework
// components. These are stub definitions that downstream changes will fill in.
package core

import (
	"fmt"
	"time"
)

// ObjectDeleted is a sentinel error returned when an object is not found.
var ObjectDeleted = fmt.Errorf("object not found")

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
	Ref          ObjectRef
	Size         int64
	ETag         string
	LastModified time.Time
}

// Hook is a lifecycle hook (before/after run, before/after agent step).
type Hook struct {
	Name   string
	Script string
}

// SandboxCaps describes what a sandbox engine can do.
type SandboxCaps struct {
	MaxMemoryMB    int
	MaxDiskMB      int
	SupportsGPU    bool
	SupportsNet    bool
	IsIsolated     bool
}
