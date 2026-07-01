package transport_test

import (
	"encoding/json"
	"testing"

	"mework/libs/shared/transport"
)

// TestDispatch_OwnerTenantRoundTrip verifies that the Dispatch wire type
// round-trips Owner, Tenant, and Session through JSON. Owner and Tenant are
// required so a runner receiving a session-open dispatch can authorize turns.
func TestDispatch_OwnerTenantRoundTrip(t *testing.T) {
	in := transport.Dispatch{
		Agent:   transport.AgentRef{Name: "code-fixer"},
		Session: "sess-123",
		Owner:   "acct-7",
		Tenant:  "tenant-9",
		Runner:  "runner-abc",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out transport.Dispatch
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Owner != "acct-7" {
		t.Errorf("Owner = %q, want %q", out.Owner, "acct-7")
	}
	if out.Tenant != "tenant-9" {
		t.Errorf("Tenant = %q, want %q", out.Tenant, "tenant-9")
	}
	if out.Session != "sess-123" {
		t.Errorf("Session = %q, want %q", out.Session, "sess-123")
	}

	// Verify JSON keys are present with the expected tags.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	for _, key := range []string{"owner", "tenant", "session"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in %s", key, data)
		}
	}
}
