package daemon

import (
	"sort"
	"strings"

	"mework/internal/mello"
)

// triggerMatch is a comment that should start an agent run.
type triggerMatch struct {
	Comment mello.Comment
}

// findTriggers returns comments that should trigger an agent run, oldest first.
// A comment triggers when ALL hold:
//   - its body contains the keyword,
//   - it was NOT authored by the daemon itself (selfUserID) — prevents the
//     daemon's own start/done comments from re-triggering an infinite loop,
//   - it has not already been handled (caller checks state).
// Ordering is by created_at so older triggers run first; comment ids may be
// UUIDs so we never rely on id ordering.
func findTriggers(comments []mello.Comment, keyword, selfUserID string) []triggerMatch {
	var matches []triggerMatch
	for _, c := range comments {
		if c.UserID != "" && c.UserID == selfUserID {
			continue // skip our own comments
		}
		if !strings.Contains(c.Body, keyword) {
			continue
		}
		matches = append(matches, triggerMatch{Comment: c})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		ci, cj := matches[i].Comment.CreatedAt, matches[j].Comment.CreatedAt
		switch {
		case ci == nil:
			return false
		case cj == nil:
			return true
		default:
			return ci.Before(*cj)
		}
	})
	return matches
}
