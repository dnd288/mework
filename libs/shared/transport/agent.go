package transport

import "encoding/json"

// Form represents the type of an agent artifact payload.
type Form string

const (
	FormDefinition Form = "definition"
	FormImage      Form = "image"
	FormBundle     Form = "bundle"
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

// Dispatch is a message published to a runner's dispatch topic. A non-empty
// Session marks an open-session dispatch (the daemon opens a long-lived
// sandbox); Owner and Tenant let the runner authorize the session's turns.
type Dispatch struct {
	Agent      AgentRef        `json:"agent"`
	Grant      json.RawMessage `json:"grant"`
	Session    string          `json:"session,omitempty"`
	Owner      string          `json:"owner,omitempty"`
	Tenant     string          `json:"tenant,omitempty"`
	Runner     string          `json:"runner"`
	ChannelKey string          `json:"channel_key,omitempty"`
	// Workspace, when set on an open-session dispatch, is an absolute local
	// directory the daemon binds the sandbox to (resolving the definition from
	// the dir's mework.yml) instead of resolving from the server catalog.
	Workspace string `json:"workspace,omitempty"`
}
