package transport

import "encoding/json"

// Form represents the type of an agent artifact payload.
type Form string

const (
	FormDefinition Form = "definition"
	FormImage      Form = "image"
)

// AgentRef identifies an agent and optionally a specific version.
type AgentRef struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Version is an immutable agent version descriptor.
type Version struct {
	Ref      AgentRef `json:"ref"`
	Form     Form     `json:"form"`
	Checksum string   `json:"checksum,omitempty"`
	Payload  []byte   `json:"payload,omitempty"`
}

// Artifact is the result of pulling an agent version.
type Artifact struct {
	Ref     AgentRef `json:"ref"`
	Form    Form     `json:"form"`
	Content []byte   `json:"content"`
}

// Dispatch is a message published to a runner's dispatch topic.
type Dispatch struct {
	Agent   AgentRef         `json:"agent"`
	Grant   json.RawMessage  `json:"grant"`
	Session string           `json:"session,omitempty"`
	Runner  string           `json:"runner"`
}
