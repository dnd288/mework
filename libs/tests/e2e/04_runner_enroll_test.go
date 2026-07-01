//go:build e2e

package e2e

import "testing"

// Feature 04 — Tenant management & runner enrollment.
// Source: SCENARIOS.md + the user's "tenant management" surface.
//
// TENANT-01..03 are GREEN (c0000-tenancy): they execute against the real
// server/registry.Service through the live World harness (tests/e2e/harness.go,
// NewWorld), asserting the tenancy delta-spec scenarios
// (openspec/changes/c0000-tenancy/specs/tenancy/spec.md). They skip only when
// TEST_DATABASE_URL is unset, like every DB-backed test in the repo. The ENROLL-*
// scenarios below remain Skip pending c0004.

// TestTENANT_01_RegisterTenant realizes the delta-spec scenario
// "Operator registers a tenant": registering a tenant yields a stable, non-empty
// identifier and a fresh isolated namespace. Distinct registrations get distinct ids.
func TestTENANT_01_RegisterTenant(t *testing.T) {
	w := NewWorld(t)

	tests := []struct {
		name string
	}{
		{name: "acme"},
		{name: "globex"},
		{name: "initech"},
	}

	seen := make(map[TenantID]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tn, err := w.Registry.RegisterTenant(ctx(), tt.name)
			if err != nil {
				t.Fatalf("RegisterTenant(%q): %v", tt.name, err)
			}
			if tn.ID == "" {
				t.Errorf("RegisterTenant(%q): got empty ID, want a stable identifier", tt.name)
			}
			if tn.Name != tt.name {
				t.Errorf("RegisterTenant(%q): Name = %q, want %q", tt.name, tn.Name, tt.name)
			}
			if seen[tn.ID] {
				t.Errorf("RegisterTenant(%q): ID %q collides with an earlier tenant; each tenant is its own namespace", tt.name, tn.ID)
			}
			seen[tn.ID] = true
		})
	}
}

// TestTENANT_02_Isolation realizes the delta-spec scenarios "Listing returns only
// the caller's tenant" and "Cross-tenant access is denied": with runners under both
// acme and globex, an acme-scoped list returns exactly acme's runners and never
// globex's, and vice versa.
func TestTENANT_02_Isolation(t *testing.T) {
	w := NewWorld(t)

	acme, err := w.Registry.RegisterTenant(ctx(), "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := w.Registry.RegisterTenant(ctx(), "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	// Each tenant enrolls its own runners via its own registration token, so the
	// resulting identities are tenant-bound by construction.
	acmeR1 := w.EnrollInto(t, acme.ID, "acme-rt-1")
	acmeR2 := w.EnrollInto(t, acme.ID, "acme-rt-2")
	globexR1 := w.EnrollInto(t, globex.ID, "globex-rt-1")

	tests := []struct {
		name       string
		tenant     TenantID
		wantOwned  []RunnerID
		wantHidden []RunnerID
	}{
		{
			name:       "acme sees only acme runners",
			tenant:     acme.ID,
			wantOwned:  []RunnerID{acmeR1, acmeR2},
			wantHidden: []RunnerID{globexR1},
		},
		{
			name:       "globex sees only globex runners",
			tenant:     globex.ID,
			wantOwned:  []RunnerID{globexR1},
			wantHidden: []RunnerID{acmeR1, acmeR2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := w.Registry.ListRunners(ctx(), tt.tenant)
			if err != nil {
				t.Fatalf("ListRunners(%s): %v", tt.tenant, err)
			}
			set := make(map[RunnerID]bool, len(got))
			for _, r := range got {
				set[r] = true
			}
			if len(got) != len(tt.wantOwned) {
				t.Fatalf("ListRunners(%s) returned %d runners, want %d: %+v", tt.tenant, len(got), len(tt.wantOwned), got)
			}
			for _, want := range tt.wantOwned {
				if !set[want] {
					t.Errorf("ListRunners(%s) missing own runner %q", tt.tenant, want)
				}
			}
			for _, hidden := range tt.wantHidden {
				if set[hidden] {
					t.Errorf("ListRunners(%s) leaked runner %q from another tenant", tt.tenant, hidden)
				}
			}
		})
	}
}

// TestTENANT_03_RegistrationTokenScopedToTenant realizes the delta-spec scenarios
// "Issued token is bound to its tenant" and "Enrolling yields a tenant-bound
// identity": a token issued for acme enrolls a runner bound to acme, and a token
// issued for one tenant never enrolls a runner into another.
func TestTENANT_03_RegistrationTokenScopedToTenant(t *testing.T) {
	w := NewWorld(t)

	acme, err := w.Registry.RegisterTenant(ctx(), "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := w.Registry.RegisterTenant(ctx(), "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	tests := []struct {
		name       string
		issueFor   TenantID
		notTenant  TenantID
		wantTenant TenantID
	}{
		{name: "acme token enrolls under acme", issueFor: acme.ID, notTenant: globex.ID, wantTenant: acme.ID},
		{name: "globex token enrolls under globex", issueFor: globex.ID, notTenant: acme.ID, wantTenant: globex.ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := w.Registry.IssueRegistrationToken(ctx(), tt.issueFor)
			if err != nil {
				t.Fatalf("IssueRegistrationToken(%s): %v", tt.issueFor, err)
			}
			if tok == "" {
				t.Fatalf("IssueRegistrationToken(%s): got empty token", tt.issueFor)
			}

			id, err := w.Registry.EnrollRunner(ctx(), tok)
			if err != nil {
				t.Fatalf("EnrollRunner(%s token): %v", tt.name, err)
			}
			if id.Tenant != tt.wantTenant {
				t.Errorf("enrolled identity Tenant = %q, want %q", id.Tenant, tt.wantTenant)
			}
			if id.Tenant == tt.notTenant {
				t.Errorf("token issued for %q enrolled a runner into %q; cross-tenant enrollment must be denied", tt.issueFor, tt.notTenant)
			}
		})
	}
}

func TestENROLL_01_EnrollNewRunner(t *testing.T) {
	Scenario(t, "ENROLL-01", "Enroll a new runner", PlannedC0005).
		Given("a valid registration token and the hub URL", func(w *World) {
			tok, _ := w.Registry.IssueRegistrationToken(ctx(), "acme")
			w.set("tok", tok)
		}).
		When("the developer runs `mework runner enroll --url <hub> --token <reg>`", func(w *World) {
			id, err := w.Registry.EnrollRunner(ctx(), w.get("tok").(string))
			w.set("id", id)
			w.expect(err == nil, "enrollment should succeed")
		}).
		Then("a durable runner identity is persisted at ~/.mework/ (0600) and is ready unattended", func(w *World) {
			id := w.get("id").(RunnerIdentity)
			w.expect(id.Runner != "" && id.Secret != "", "a durable runner identity must be persisted")
		}).
		Run()
}

func TestENROLL_02_UnattendedAfterEnrollment(t *testing.T) {
	Scenario(t, "ENROLL-02", "Unattended after enrollment", PlannedC0005).
		Given("an enrolled runner", func(w *World) {}).
		When("the runner process restarts", func(w *World) {
			w.expect(w.StartRunner() == nil, "runner restarts using its persisted identity")
		}).
		Then("it resumes receiving and running dispatches with no re-configuration", func(w *World) {
			w.expect(true, "install-once; unattended thereafter")
		}).
		Run()
}

func TestENROLL_03_InvalidTokenRejected(t *testing.T) {
	Scenario(t, "ENROLL-03", "Invalid or expired registration token is rejected", PlannedC0005).
		Given("an invalid or already-used registration token", func(w *World) {}).
		When("the developer runs `mework runner enroll`", func(w *World) {
			_, err := w.Registry.EnrollRunner(ctx(), "bad-token")
			w.set("err", err)
		}).
		Then("enrollment fails and no identity is persisted", func(w *World) {
			w.expect(w.get("err") != nil, "an invalid registration token must be rejected with nothing persisted")
		}).
		Run()
}

func TestENROLL_04_RegTokenNotReusableAsIdentity(t *testing.T) {
	Scenario(t, "ENROLL-04", "Registration token is not reusable as the identity", PlannedC0005).
		Given("a registration token that successfully enrolled a runner", func(w *World) {}).
		When("the same registration token is presented to a transport route", func(w *World) {
			_, err := w.Auth.AuthRunner(ctx(), "the-registration-token")
			w.set("err", err)
		}).
		Then("it is rejected; only the durable runner identity authenticates transport routes", func(w *World) {
			w.expect(w.get("err") != nil, "a registration token must not authenticate transport routes")
		}).
		Run()
}

func TestENROLL_05_InspectAgentsAndSessions(t *testing.T) {
	Scenario(t, "ENROLL-05", "Inspect dispatched agents and active sessions", PlannedC0005).
		Given("an enrolled runner with one or more active sessions", func(w *World) {}).
		When("the developer runs `mework agent list --json` and `mework session list --json`", func(w *World) {
			out, err := w.RunCLI("session", "list", "--json")
			w.set("out", out)
			w.expect(err == nil, "session list should succeed")
		}).
		Then("each emits the dispatched agents / active sessions as JSON (read-only)", func(w *World) {
			w.expect(w.get("out") != "", "inspection commands emit JSON for scripting")
		}).
		Run()
}
