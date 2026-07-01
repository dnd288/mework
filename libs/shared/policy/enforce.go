package policy

import (
	"fmt"
)

// Enforce evaluates all rules in order against the given attributes.
// The first matching rule determines the result. If no rule matches,
// the policy's default action applies.
//
// For rate_limit rules, the caller must check the rate limiter separately
// after Enforce returns ActionRateLimit.
func (p *Policy) Enforce(attrs Attributes) (*Result, error) {
	if p == nil {
		return &Result{Allowed: true}, nil
	}

	for _, rule := range p.Rules {
		match, err := ruleMatches(rule, attrs)
		if err != nil {
			return nil, fmt.Errorf("policy rule error: %w", err)
		}
		if !match {
			continue
		}

		switch rule.Action {
		case ActionAllow:
			return &Result{Allowed: true}, nil
		case ActionDeny:
			reason := rule.Reason
			if reason == "" {
				reason = "blocked by policy"
			}
			return &Result{Allowed: false, Reason: reason}, nil
		case ActionRateLimit:
			// Return a special result; caller checks the rate limiter.
			return &Result{Allowed: true, Reason: rule.Limit}, nil
		default:
			return nil, fmt.Errorf("unknown action %q", rule.Action)
		}
	}

	// No rule matched — use default.
	switch p.DefaultAction() {
	case ActionDeny:
		return &Result{Allowed: false, Reason: "denied by default policy"}, nil
	default:
		return &Result{Allowed: true}, nil
	}
}

// ruleMatches checks whether ALL match conditions in a rule are satisfied
// by the given attributes (AND semantics).
func ruleMatches(rule Rule, attrs Attributes) (bool, error) {
	if len(rule.Match) == 0 {
		// Empty match = matches everything.
		return true, nil
	}

	for attr, pattern := range rule.Match {
		value := attrs[attr]
		matched, err := matchAttribute(value, pattern)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}
