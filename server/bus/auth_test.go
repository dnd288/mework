package bus_test

import (
	"testing"

	"mework/server/bus"
)

// TestAuthorizeTopics verifies that AuthorizeTopics correctly filters topics
// based on the subscriber's runtime identity, matching the message-bus
// delta-spec subscription authorization requirements (BUS-08).
func TestAuthorizeTopics(t *testing.T) {
	tests := []struct {
		name           string
		runtimeID      string
		topics         []bus.Topic
		want           []bus.Topic
		wantErr        bool
		wantErrIsEmpty bool // if true, error is expected when no topics match
	}{
		{
			name:      "authorized topic accepted",
			runtimeID: "R",
			topics:    []bus.Topic{bus.Topic("runner.R.dispatch")},
			want:      []bus.Topic{bus.Topic("runner.R.dispatch")},
		},
		{
			name:           "unauthorized topic rejected BUS-08",
			runtimeID:      "R",
			topics:         []bus.Topic{bus.Topic("runner.OTHER.dispatch")},
			wantErr:        true,
			wantErrIsEmpty: true,
		},
		{
			name:      "multiple topics partial auth",
			runtimeID: "R",
			topics: []bus.Topic{
				bus.Topic("runner.R.dispatch"),
				bus.Topic("runner.OTHER.dispatch"),
			},
			want: []bus.Topic{bus.Topic("runner.R.dispatch")},
		},
		{
			name:      "owner session control topic is authorized",
			runtimeID: "R",
			topics:    []bus.Topic{bus.Topic("session.s1.control")},
			want:      []bus.Topic{bus.Topic("session.s1.control")},
		},
		{
			name:      "non-owner session control topic is rejected",
			runtimeID: "R",
			topics:    []bus.Topic{bus.Topic("session.s2.control")},
			wantErr:   true,
		},
		{
			name:           "empty requested topics returns empty",
			runtimeID:      "R",
			topics:         nil,
			want:           nil,
		},
		{
			name:      "wildcard topic runner.<id>.* is authorized",
			runtimeID: "R",
			topics:    []bus.Topic{bus.Topic("runner.R.dispatch"), bus.Topic("runner.R.status")},
			want:      []bus.Topic{bus.Topic("runner.R.dispatch"), bus.Topic("runner.R.status")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bus.AuthorizeTopics(tt.runtimeID, tt.topics)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil with result %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("AuthorizeTopics: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d topics, want %d: got %v, want %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("topic[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
