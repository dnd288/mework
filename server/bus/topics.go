package bus

import "strings"

const (
	// TopicRunnerDispatch is the topic template for dispatching a runner.
	// Use FormatTopic to produce "runner.<id>.dispatch".
	TopicRunnerDispatch = "runner.%s.dispatch"

	// TopicSessionControl is the topic template for session control messages.
	// Use FormatTopic to produce "session.<id>.control".
	TopicSessionControl = "session.%s.control"

	// TopicWildcard matches any topic.
	TopicWildcard Topic = "*"
)

// FormatTopic substitutes %s in the template with the given id.
func FormatTopic(tmpl string, id string) Topic {
	return Topic(strings.Replace(tmpl, "%s", id, 1))
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
