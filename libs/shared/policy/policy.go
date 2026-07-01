// Package policy provides an attribute-based message policy engine for agent
// message routing. Agents declare rules that match on message attributes
// (sender, channel, time, content, etc.) and specify an action (allow, deny,
// rate_limit). The first matching rule wins; if no rule matches, a configurable
// default action applies.
package policy

import (
	"fmt"
	"strings"
)

// Action is the action a rule takes when its match conditions are met.
type Action string

const (
	ActionAllow      Action = "allow"
	ActionDeny       Action = "deny"
	ActionRateLimit  Action = "rate_limit"
)

// Rule is a single policy rule. Match contains attribute→pattern pairs that
// MUST ALL match (AND semantics) for the rule to fire. When the rule fires,
// the Action is taken.
type Rule struct {
	Match  map[string]string `yaml:"match" json:"match"`
	Action Action            `yaml:"action" json:"action"`
	Reason string            `yaml:"reason,omitempty" json:"reason,omitempty"`
	Limit  string            `yaml:"limit,omitempty" json:"limit,omitempty"` // e.g. "30/m"
}

// Policy is the complete message policy for an agent. Rules are evaluated in
// order; the first matching rule wins.
type Policy struct {
	Rules   []Rule `yaml:"rules,omitempty" json:"rules,omitempty"`
	Default Action `yaml:"default,omitempty" json:"default,omitempty"` // "allow" (default) or "deny"
}

// Default returns the default action, defaulting to ActionAllow.
func (p *Policy) DefaultAction() Action {
	if p.Default == "" {
		return ActionAllow
	}
	return p.Default
}

// Attributes is a set of key-value pairs describing a message at the
// enforcement point. Keys are case-sensitive.
type Attributes map[string]string

// Result carries the enforcement outcome.
type Result struct {
	Allowed bool
	Reason  string // non-empty when denied
}

// String returns a human-readable denial reason.
func (r *Result) String() string {
	if r.Allowed {
		return "allowed"
	}
	return fmt.Sprintf("denied: %s", r.Reason)
}

// ParseLimit parses a rate-limit string like "30/m" into count and period.
func ParseLimit(s string) (count int, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "/m") {
		return 0, false
	}
	n := 0
	if _, err := fmt.Sscanf(s, "%d/m", &n); err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
