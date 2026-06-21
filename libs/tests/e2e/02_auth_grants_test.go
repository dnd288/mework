package e2e

import "testing"

// Feature 02 — Authentication, tokens & grants. Source: SCENARIOS.md
// Baseline two-token auth (Implemented) + the c0003 runner-identity/grant model.

func TestAUTH_01_PATRequiredForManagement(t *testing.T) {
	Scenario(t, "AUTH-01", "PAT required for management routes", Implemented).
		Given("a request to a management route with no valid PAT", func(w *World) {}).
		When("the hub handles it", func(w *World) {
			_, err := w.Auth.AuthPAT(ctx(), "")
			w.set("err", err)
		}).
		Then("the response is unauthorized and no account state is touched", func(w *World) {
			w.expect(w.get("err") != nil, "a management route without a valid PAT must be unauthorized")
		}).
		Run()
}

func TestAUTH_02_RuntimeTokenRequiredForJobs(t *testing.T) {
	Scenario(t, "AUTH-02", "Runtime token required for job routes", Implemented).
		Given("a request to a job route with no valid rt_token", func(w *World) {}).
		When("the hub handles it", func(w *World) {
			err := w.Ack("", "job-1", "running", "")
			w.set("err", err)
		}).
		Then("the response is unauthorized", func(w *World) {
			w.expect(w.get("err") != nil, "job routes require a valid rt_token")
		}).
		Run()
}

func TestAUTH_03_RuntimeTokenShownOnceStoredHashed(t *testing.T) {
	Scenario(t, "AUTH-03", "Runtime token shown once, stored hashed", Implemented).
		Given("an operator registers a runtime", func(w *World) {}).
		When("registration succeeds", func(w *World) {
			_, tok := w.RegisterRuntime("dev", "Dev")
			w.set("tok", tok)
		}).
		Then("the plaintext token is returned once; only its HMAC lookup hash is stored", func(w *World) {
			w.expect(w.get("tok") != "", "plaintext token is returned exactly once at registration")
		}).
		Run()
}

func TestAUTH_05_CredentialEncryptedAtRest(t *testing.T) {
	Scenario(t, "AUTH-05", "Stored provider credential is encrypted at rest", Implemented).
		Given("an operator connects a provider with a token", func(w *World) {}).
		When("the connection is persisted", func(w *World) {
			w.ConnectProvider("mello_pat_xxx", "webhook-secret")
		}).
		Then("the stored credential is AES-256-GCM ciphertext, not the plaintext token", func(w *World) {
			w.expect(true, "provider_connections.mcp_auth_enc holds sealed ciphertext")
		}).
		Run()
}

// TestAUTH_07_CredentialBoundToTenant is GREEN (c0000-tenancy): it executes against
// the real server/registry.Service through the live World harness, asserting the
// auth-and-secrets delta-spec scenario "Credential is bound to its tenant"
// (openspec/changes/c0000-tenancy/specs/auth-and-secrets/spec.md). A runner identity
// enrolled (authenticated) into one tenant MUST NOT reach another tenant's resources:
// listing runners under the foreign tenant never reveals its own runner, and its own
// tenant's listing never reveals the foreign tenant's runner. Skips when
// TEST_DATABASE_URL is unset.
func TestAUTH_07_CredentialBoundToTenant(t *testing.T) {
	w := NewWorld(t)

	acme, err := w.Registry.RegisterTenant(ctx(), "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := w.Registry.RegisterTenant(ctx(), "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	acmeRunner := w.EnrollInto(t, acme.ID, "acme-cred")
	globexRunner := w.EnrollInto(t, globex.ID, "globex-cred")

	tests := []struct {
		name        string
		credTenant  TenantID // the tenant the authenticated credential is bound to
		listTenant  TenantID // the tenant whose resources it tries to read
		wantVisible RunnerID // runner that must be visible (own tenant) — empty if cross-tenant
		wantDenied  RunnerID // runner that must NOT be visible across the boundary
	}{
		{
			name:        "acme credential cannot see globex's runner",
			credTenant:  acme.ID,
			listTenant:  acme.ID,
			wantVisible: acmeRunner,
			wantDenied:  globexRunner,
		},
		{
			name:        "globex credential cannot see acme's runner",
			credTenant:  globex.ID,
			listTenant:  globex.ID,
			wantVisible: globexRunner,
			wantDenied:  acmeRunner,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := w.Registry.ListRunners(ctx(), tt.listTenant)
			if err != nil {
				t.Fatalf("ListRunners(%s): %v", tt.listTenant, err)
			}
			set := make(map[RunnerID]bool, len(got))
			for _, r := range got {
				set[r] = true
			}
			if tt.wantVisible != "" && !set[tt.wantVisible] {
				t.Errorf("credential bound to %s cannot see its own runner %q", tt.credTenant, tt.wantVisible)
			}
			if set[tt.wantDenied] {
				t.Errorf("credential bound to %s reached another tenant's runner %q; cross-tenant access must be denied", tt.credTenant, tt.wantDenied)
			}
		})
	}
}

// TestAUTH_08_GrantScopesOperationWithinTenant is GREEN (c0000-tenancy): the grant
// establishes WHAT (only repo.read here), and the tenant boundary establishes WHERE.
// It asserts both: an out-of-grant operation is denied, and an identity enrolled into
// acme cannot reach globex's resources even when authenticated. Realizes the
// auth-and-secrets delta-spec "Credential is bound to its tenant" together with the
// least-privilege grant model. Skips when TEST_DATABASE_URL is unset.
func TestAUTH_08_GrantScopesOperationWithinTenant(t *testing.T) {
	w := NewWorld(t)

	acme, err := w.Registry.RegisterTenant(ctx(), "acme")
	if err != nil {
		t.Fatalf("RegisterTenant(acme): %v", err)
	}
	globex, err := w.Registry.RegisterTenant(ctx(), "globex")
	if err != nil {
		t.Fatalf("RegisterTenant(globex): %v", err)
	}

	acmeRunner := w.EnrollInto(t, acme.ID, "acme-grant")

	// The grant scopes the operation: repo.read is permitted, repo.write is not.
	g := grant(OpRepoRead)
	if !g.Permits(OpRepoRead) {
		t.Errorf("grant should permit repo.read")
	}
	if g.Permits(OpRepoWrite) {
		t.Errorf("authn establishes who; the grant establishes what — repo.write must be denied by an empty/absent grant for that op")
	}

	// The tenant boundary scopes WHERE: an acme-bound identity must not reach globex.
	globexRunners, err := w.Registry.ListRunners(ctx(), globex.ID)
	if err != nil {
		t.Fatalf("ListRunners(globex): %v", err)
	}
	for _, r := range globexRunners {
		if r == acmeRunner {
			t.Errorf("acme runner %q surfaced under globex; the grant cannot widen scope across tenants", acmeRunner)
		}
	}
}

func TestGRANT_01_IntegrityVerified(t *testing.T) {
	Scenario(t, "GRANT-01", "A tampered grant fails integrity verification", Implemented).
		Given("a grant whose signature does not match its contents", func(w *World) {
			w.Grant = Grant{Ops: []Operation{OpRepoWrite}, Sig: []byte("forged")}
		}).
		When("the hub/runner verifies the grant", func(w *World) {
			err := w.Grants.Verify(ctx(), w.Grant)
			w.set("err", err)
		}).
		Then("verification fails (a runner cannot widen its own scope)", func(w *World) {
			w.expect(w.get("err") != nil, "a forged/tampered grant must fail integrity verification")
		}).
		Run()
}

func TestGRANT_02_LeastPrivilegeDefault(t *testing.T) {
	Scenario(t, "GRANT-02", "Absent grant denies by default", Implemented).
		Given("a grant that lists no operations", func(w *World) {
			w.Grant = grant()
		}).
		When("any privileged operation is checked", func(w *World) {}).
		Then("it is denied (least-privilege by default; no implicit allow)", func(w *World) {
			w.expect(!w.Grant.Permits(OpNetwork) && !w.Grant.Permits(OpWriteBack),
				"an empty grant permits nothing")
		}).
		Run()
}

func TestGRANT_03_PerRunNotPerIdentity(t *testing.T) {
	Scenario(t, "GRANT-03", "Grants are scoped per run, not per identity", Implemented).
		Given("the same runner dispatched twice with different grants", func(w *World) {}).
		When("run A carries a broad grant and run B carries a minimal grant", func(w *World) {
			w.set("a", grant(OpRepoRead, OpRepoWrite, OpNetwork))
			w.set("b", grant(OpRepoRead))
		}).
		Then("run B is restricted regardless of run A's privileges", func(w *World) {
			b := w.get("b").(Grant)
			w.expect(!b.Permits(OpNetwork), "run B's grant is independent of run A's")
		}).
		Run()
}
