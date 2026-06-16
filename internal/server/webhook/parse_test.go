package webhook

import (
	"testing"
)

func TestParseTrigger(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantProfile  string
		wantWorkflow string
		wantInst     string
		wantOk       bool
	}{
		{
			name:         "valid trigger with profile and workflow",
			body:         "@mework dev review fix the login bug",
			wantProfile:  "dev",
			wantWorkflow: "review",
			wantInst:     "fix the login bug",
			wantOk:       true,
		},
		{
			name:         "valid trigger with profile only",
			body:         "@mework dev fix it",
			wantProfile:  "dev",
			wantWorkflow: "",
			wantInst:     "fix it",
			wantOk:       true,
		},
		{
			name:         "valid trigger with leading whitespace",
			body:         "   @mework dev review fix it",
			wantProfile:  "dev",
			wantWorkflow: "review",
			wantInst:     "fix it",
			wantOk:       true,
		},
		{
			name:         "valid trigger with newlines and multiple spaces",
			body:         "@mework   dev   review\n  line1\n  line2",
			wantProfile:  "dev",
			wantWorkflow: "review",
			wantInst:     "line1\n  line2",
			wantOk:       true,
		},
		{
			name:         "no trigger present",
			body:         "just a regular comment without the tag",
			wantProfile:  "",
			wantWorkflow: "",
			wantInst:     "",
			wantOk:       false,
		},
		{
			name:         "tag inside an email should be ignored",
			body:         "my email is test@mework.com",
			wantProfile:  "",
			wantWorkflow: "",
			wantInst:     "",
			wantOk:       false,
		},
		{
			name:         "valid trigger after some text",
			body:         "Please do this:\n@mework prod ship code",
			wantProfile:  "prod",
			wantWorkflow: "ship",
			wantInst:     "code",
			wantOk:       true,
		},
		{
			name:         "valid trigger with profile name only, no prompt",
			body:         "@mework staging",
			wantProfile:  "staging",
			wantWorkflow: "",
			wantInst:     "",
			wantOk:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, workflow, inst, ok := ParseTrigger(tt.body)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile, tt.wantProfile)
			}
			if workflow != tt.wantWorkflow {
				t.Errorf("workflow = %q, want %q", workflow, tt.wantWorkflow)
			}
			if inst != tt.wantInst {
				t.Errorf("instructions = %q, want %q", inst, tt.wantInst)
			}
		})
	}
}
