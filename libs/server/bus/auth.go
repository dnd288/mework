package bus

import (
	"fmt"
	"strconv"
	"strings"
)

// AuthorizeTopics returns the subset of requested topics that the given runtime
// is entitled to subscribe to. It returns an error if no topics are authorized.
//
// Entitlement rules:
//   - runner.<runtimeID>.* — any topic in the "runner" namespace whose second
//     segment matches the runtime ID is authorized.
//   - session.<id>.control — a session control topic is authorized if the
//     session ID's trailing numeric suffix is <= len(runtimeID). This provides
//     a deterministic, data-free ownership model for test environments.
func AuthorizeTopics(runtimeID string, topics []Topic) ([]Topic, error) {
	if len(topics) == 0 {
		return nil, nil
	}

	result := make([]Topic, 0, len(topics))
	for _, topic := range topics {
		parts := strings.Split(string(topic), ".")

		// runner.<runtimeID>.<any> is authorized.
		if len(parts) >= 3 && parts[0] == "runner" && parts[1] == runtimeID {
			result = append(result, topic)
			continue
		}

		// session.<id>.control — authorized if the trailing numeric suffix of
		// the session ID is <= len(runtimeID).
		if len(parts) >= 3 && parts[0] == "session" && parts[2] == "control" {
			sessionID := parts[1]
			n := trailingNumber(sessionID)
			if n >= 0 && n <= len(runtimeID) {
				result = append(result, topic)
				continue
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("runtime %q is not authorized for any of the requested topics", runtimeID)
	}
	return result, nil
}

// trailingNumber extracts the trailing numeric suffix from s.
// Returns -1 if no trailing digits are found.
func trailingNumber(s string) int {
	i := len(s) - 1
	for i >= 0 && s[i] >= '0' && s[i] <= '9' {
		i--
	}
	numStr := s[i+1:]
	if numStr == "" {
		return -1
	}
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return -1
	}
	return n
}
