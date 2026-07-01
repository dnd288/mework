package policy

import (
	"testing"
)

func TestEnforce_NilPolicy(t *testing.T) {
	r, err := (*Policy)(nil).Enforce(Attributes{"sender": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Allowed {
		t.Errorf("nil policy should allow all, got denied: %s", r.Reason)
	}
}

func TestEnforce_EmptyPolicy(t *testing.T) {
	p := &Policy{}
	r, err := p.Enforce(Attributes{"sender": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Allowed {
		t.Errorf("empty policy should allow all, got denied: %s", r.Reason)
	}
}

func TestEnforce_DenyBySender(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{Match: map[string]string{"sender": "bob"}, Action: ActionDeny, Reason: "no bob"},
		},
	}

	r, err := p.Enforce(Attributes{"sender": "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Allowed {
		t.Errorf("expected deny for bob")
	}
	if r.Reason != "no bob" {
		t.Errorf("expected reason 'no bob', got %q", r.Reason)
	}

	// alice should be allowed (no matching rule)
	r, err = p.Enforce(Attributes{"sender": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Allowed {
		t.Errorf("expected allow for alice")
	}
}

func TestEnforce_DenyDefault(t *testing.T) {
	p := &Policy{
		Default: ActionDeny,
		Rules: []Rule{
			{Match: map[string]string{"sender": "admin"}, Action: ActionAllow},
		},
	}

	r, err := p.Enforce(Attributes{"sender": "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Allowed {
		t.Errorf("expected allow for admin")
	}

	r, err = p.Enforce(Attributes{"sender": "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Allowed {
		t.Errorf("expected deny for unknown with deny default")
	}
}

func TestEnforce_FirstMatchWins(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{Match: map[string]string{"sender": "*"}, Action: ActionDeny, Reason: "block all"},
			{Match: map[string]string{"sender": "admin"}, Action: ActionAllow},
		},
	}

	r, _ := p.Enforce(Attributes{"sender": "admin"})
	// First rule matches everything, so admin should be denied too
	if r.Allowed {
		t.Errorf("first-match-wins: admin should be denied by first rule")
	}
}

func TestEnforce_MultiAttributeAND(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{
				Match:  map[string]string{"sender": "alice", "authenticated": "true"},
				Action: ActionAllow,
			},
			{
				Match:  map[string]string{"sender": "alice", "authenticated": "false"},
				Action: ActionDeny,
				Reason: "unauthenticated",
			},
		},
	}

	// alice authenticated → allow
	r, _ := p.Enforce(Attributes{"sender": "alice", "authenticated": "true"})
	if !r.Allowed {
		t.Errorf("expected allow for authenticated alice")
	}

	// alice unauthenticated → deny
	r, _ = p.Enforce(Attributes{"sender": "alice", "authenticated": "false"})
	if r.Allowed {
		t.Errorf("expected deny for unauthenticated alice")
	}
}

func TestEnforce_ContentFilter(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{
				Match:  map[string]string{"content": "rm -rf /*"},
				Action: ActionDeny,
				Reason: "blocked dangerous command",
			},
		},
	}

	r, _ := p.Enforce(Attributes{"content": "rm -rf /"})
	if r.Allowed {
		t.Errorf("expected deny for dangerous content")
	}
}

func TestEnforce_RegexContent(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{
				Match:  map[string]string{"content": "/^rm .+/"},
				Action: ActionDeny,
				Reason: "blocked rm command",
			},
		},
	}

	r, _ := p.Enforce(Attributes{"content": "rm -rf /"})
	if r.Allowed {
		t.Errorf("expected deny for rm command")
	}

	r, _ = p.Enforce(Attributes{"content": "ls -la"})
	if !r.Allowed {
		t.Errorf("expected allow for ls command")
	}
}

func TestEnforce_ContentLength(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{
				Match:  map[string]string{"content_length": ">100"},
				Action: ActionDeny,
				Reason: "too long",
			},
		},
	}

	r, _ := p.Enforce(Attributes{"content_length": "101"})
	if r.Allowed {
		t.Errorf("expected deny for content_length > 100")
	}

	r, _ = p.Enforce(Attributes{"content_length": "50"})
	if !r.Allowed {
		t.Errorf("expected allow for content_length <= 100")
	}
}

func TestEnforce_TimeRange(t *testing.T) {
	// With no time specified, matchTimeRange uses time.Now().UTC().
	// This test uses an explicit time attribute to be deterministic.
	p := &Policy{
		Rules: []Rule{
			{
				Match:  map[string]string{"time": "22:00-06:00"},
				Action: ActionDeny,
				Reason: "agent sleeping",
			},
		},
	}

	// 03:00 UTC is within 22:00-06:00 (wraps past midnight)
	r, _ := p.Enforce(Attributes{"time": "2026-06-30T03:00:00Z"})
	if r.Allowed {
		t.Errorf("expected deny during sleep hours (03:00 UTC)")
	}

	// 12:00 UTC is outside 22:00-06:00
	r, _ = p.Enforce(Attributes{"time": "2026-06-30T12:00:00Z"})
	if !r.Allowed {
		t.Errorf("expected allow during daytime (12:00 UTC)")
	}
}

func TestEnforce_RateLimitAction(t *testing.T) {
	p := &Policy{
		Rules: []Rule{
			{
				Match:  map[string]string{"sender": "*"},
				Action: ActionRateLimit,
				Limit:  "30/m",
			},
		},
	}

	r, _ := p.Enforce(Attributes{"sender": "alice"})
	if !r.Allowed {
		t.Errorf("rate_limit action should be Allowed=true (caller checks limiter)")
	}
	if r.Reason != "30/m" {
		t.Errorf("expected limit '30/m' in reason, got %q", r.Reason)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter()

	// Allow 3 per minute.
	if !rl.Allow("alice", 3) {
		t.Error("1st message should be allowed")
	}
	if !rl.Allow("alice", 3) {
		t.Error("2nd message should be allowed")
	}
	if !rl.Allow("alice", 3) {
		t.Error("3rd message should be allowed")
	}
	if rl.Allow("alice", 3) {
		t.Error("4th message should be rate-limited")
	}

	// Different sender should not be affected.
	if !rl.Allow("bob", 3) {
		t.Error("bob's 1st message should be allowed")
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		input string
		count int
		ok    bool
	}{
		{"30/m", 30, true},
		{"1/m", 1, true},
		{"", 0, false},
		{"30/s", 0, false},
		{"abc", 0, false},
	}
	for _, tt := range tests {
		count, ok := ParseLimit(tt.input)
		if ok != tt.ok || count != tt.count {
			t.Errorf("ParseLimit(%q) = (%d, %v), want (%d, %v)", tt.input, count, ok, tt.count, tt.ok)
		}
	}
}

func TestMatchAttribute_Glob(t *testing.T) {
	matched, err := matchAttribute("alice", "dev-*")
	if err != nil {
		t.Fatal(err)
	}
	if matched {
		t.Errorf("'alice' should not match 'dev-*'")
	}

	matched, err = matchAttribute("dev-alice", "dev-*")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Errorf("'dev-alice' should match 'dev-*'")
	}
}

func TestMatchAttribute_Regex(t *testing.T) {
	matched, err := matchAttribute("rm -rf /", "/^rm .+/")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Errorf("'rm -rf /' should match '/^rm .+/'")
	}

	matched, err = matchAttribute("ls -la", "/^rm .+/")
	if err != nil {
		t.Fatal(err)
	}
	if matched {
		t.Errorf("'ls -la' should not match '/^rm .+/'")
	}
}

func TestMatchAttribute_Wildcard(t *testing.T) {
	matched, err := matchAttribute("anything", "*")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Errorf("wildcard should match anything")
	}
}
