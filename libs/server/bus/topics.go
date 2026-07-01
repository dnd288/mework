package bus

import (
	"fmt"
	"strings"
)

const (
	// TopicRunnerDispatch is the topic template for dispatching a runner.
	// Use FormatTopic to produce "runner.<id>.dispatch".
	TopicRunnerDispatch = "runner.%s.dispatch"

	// TopicSessionControl is the topic template for the session's outgoing
	// (runner → hub) event channel. Use FormatTopic to produce
	// "session.<id>.control". The runner publishes ChatEvents here and the hub
	// subscribes and relays them to session subscribers.
	TopicSessionControl = "session.%s.control"

	// TopicSessionInput is the topic template for the session's inbound
	// (hub → runner) channel. Use FormatTopic to produce "session.<id>.input".
	// The hub publishes chat turns and control messages (cancel/close) here and
	// the running sandbox/agent subscribes to it. Keeping input and control as
	// single-direction topics prevents the daemon from receiving its own events.
	TopicSessionInput = "session.%s.input"

	// TopicWildcard matches any topic.
	TopicWildcard Topic = "*"
)

// FormatTopic substitutes %s in the template with the given id.
func FormatTopic(tmpl string, id string) Topic {
	return Topic(strings.Replace(tmpl, "%s", id, 1))
}

// FormatChannelTopic formats a channel event topic from its components.
// Produces "channel.<providerCode>.<resourceID>.<eventType>".
func FormatChannelTopic(providerCode, resourceID, eventType string) Topic {
	return Topic(fmt.Sprintf("channel.%s.%s.%s", providerCode, resourceID, eventType))
}

// TopicChannelEvent is a helper to format a channel event topic string.
// Identical to FormatChannelTopic; provided for parallel naming.
func TopicChannelEvent(providerCode, resourceID, eventType string) Topic {
	return FormatChannelTopic(providerCode, resourceID, eventType)
}

// MatchTopic reports whether the pattern matches the actual topic name.
// The pattern supports single-segment wildcard (*) matching exactly one
// dot-separated segment, and double-wildcard (**) matching any remaining
// segments. Exact segments must match literally.
func MatchTopic(pattern Filter, actual Topic) bool {
	p := string(pattern)
	a := string(actual)

	if p == "*" || p == "**" || p == a {
		return true
	}

	pParts := strings.Split(p, ".")
	aParts := strings.Split(a, ".")

	return matchSegments(pParts, aParts)
}

func matchSegments(pattern, actual []string) bool {
	pi, ai := 0, 0
	for pi < len(pattern) && ai < len(actual) {
		if pattern[pi] == "**" {
			return true
		}
		if pattern[pi] != "*" && pattern[pi] != actual[ai] {
			return false
		}
		pi++
		ai++
	}
	return pi == len(pattern) && ai == len(actual)
}
