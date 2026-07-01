package bus

import (
	"testing"
)

// TestMatchTopic_ChannelPatterns verifies that MatchTopic correctly handles
// channel-scoped topic patterns with segment wildcard (*) support.
// Delta-spec scenario: "Sandbox subscribes to its channel".
func TestMatchTopic_ChannelPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern Filter
		topic   Topic
		match   bool
	}{
		// Channel.* pattern matches specific event types on the same resource
		{
			name:    "channel.* matches dispatch event",
			pattern: Filter("channel.mello.TICKET-99.*"),
			topic:   Topic("channel.mello.TICKET-99.dispatch"),
			match:   true,
		},
		{
			name:    "channel.* matches control event",
			pattern: Filter("channel.mello.TICKET-99.*"),
			topic:   Topic("channel.mello.TICKET-99.control"),
			match:   true,
		},
		{
			name:    "channel.* matches status event",
			pattern: Filter("channel.mello.TICKET-99.*"),
			topic:   Topic("channel.mello.TICKET-99.status"),
			match:   true,
		},
		// Different resource ID — should NOT match
		{
			name:    "channel.* does not match different resource",
			pattern: Filter("channel.mello.TICKET-99.*"),
			topic:   Topic("channel.mello.TICKET-98.dispatch"),
			match:   false,
		},
		// Different provider — should NOT match
		{
			name:    "channel.* does not match different provider",
			pattern: Filter("channel.mello.TICKET-99.*"),
			topic:   Topic("channel.github.42.dispatch"),
			match:   false,
		},
		// Double wildcard matches all remaining segments
		{
			name:    "channel.** matches any channel topic",
			pattern: Filter("channel.**"),
			topic:   Topic("channel.mello.TICKET-99.dispatch"),
			match:   true,
		},
		{
			name:    "channel.** matches deep topic",
			pattern: Filter("channel.**"),
			topic:   Topic("channel.mello.TICKET-99.control.sub"),
			match:   true,
		},
		// FormatChannelTopic helper produces correct topic strings
		{
			name:    "FormatChannelTopic produces matching topic",
			pattern: Filter("channel.mello.TICKET-99.*"),
			topic:   FormatChannelTopic("mello", "TICKET-99", "dispatch"),
			match:   true,
		},
		// TopicChannelEvent constant matches FormatChannelTopic output
		{
			name:    "TopicChannelEvent template matches formatted topic",
			pattern: Filter(string(FormatChannelTopic("mello", "TICKET-99", "dispatch"))),
			topic:   TopicChannelEvent("mello", "TICKET-99", "dispatch"),
			match:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchTopic(tt.pattern, tt.topic)
			if got != tt.match {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, got, tt.match)
			}
		})
	}
}

// TestMatchTopic_ChannelIsolation verifies that a channel-specific filter does
// not leak events to other channels on the same worker. Delta-spec scenario:
// "Channel isolation".
func TestMatchTopic_ChannelIsolation(t *testing.T) {
	tests := []struct {
		name    string
		filter  Filter
		topics  []Topic
		matched []bool // one per topic
	}{
		{
			name:   "channel filter isolates by provider",
			filter: Filter("channel.mello.TICKET-99.*"),
			topics: []Topic{
				Topic("channel.mello.TICKET-99.dispatch"),
				Topic("channel.github.42.dispatch"),
				Topic("channel.mello.TICKET-98.dispatch"),
			},
			matched: []bool{true, false, false},
		},
		{
			name:   "github channel isolates from mello",
			filter: Filter("channel.github.42.*"),
			topics: []Topic{
				Topic("channel.github.42.dispatch"),
				Topic("channel.mello.TICKET-99.dispatch"),
			},
			matched: []bool{true, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, topic := range tt.topics {
				got := MatchTopic(tt.filter, topic)
				if got != tt.matched[i] {
					t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.filter, topic, got, tt.matched[i])
				}
			}
		})
	}
}
