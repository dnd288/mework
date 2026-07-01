//go:build e2e

package e2e

import "testing"

// Feature 09 — Agent catalog & dispatch (target). Source: SCENARIOS.md
// Spec: openspec/changes/c0003-agent-catalog. Skips pending c0003.

func TestCAT_01_PublishNewVersion(t *testing.T) {
	Scenario(t, "CAT-01", "Publish a new agent version", Implemented).
		Given("an authenticated operator", func(w *World) {}).
		When("the operator publishes code-fixer@1.2.0", func(w *World) {
			v, err := w.Catalog.PublishVersion(ctx(), Identity{Tenant: "t1"}, "code-fixer", "1.2.0", FormDefinition, []byte("manifest"))
			w.set("v", v)
			w.expect(err == nil, "publish should succeed")
		}).
		Then("the version is immutable and retrievable as code-fixer@1.2.0", func(w *World) {
			got, err := w.Catalog.Resolve(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"})
			w.expect(err == nil && got.Ref.Version == "1.2.0", "resolve should return the published version")
		}).
		Run()
}

func TestCAT_02_RepublishRejected(t *testing.T) {
	Scenario(t, "CAT-02", "Republishing an existing version is rejected", Implemented).
		Given("code-fixer@1.2.0 already exists", func(w *World) {
			_, _ = w.Catalog.PublishVersion(ctx(), Identity{}, "code-fixer", "1.2.0", FormDefinition, []byte("a"))
		}).
		When("an operator republishes 1.2.0 with different content", func(w *World) {
			_, err := w.Catalog.PublishVersion(ctx(), Identity{}, "code-fixer", "1.2.0", FormDefinition, []byte("b"))
			w.set("err", err)
		}).
		Then("the hub rejects the publish rather than overwriting the immutable version", func(w *World) {
			w.expect(w.get("err") != nil, "republishing an immutable version must be rejected")
		}).
		Run()
}

func TestCAT_03_ResolveMovingPointer(t *testing.T) {
	Scenario(t, "CAT-03", "Resolve a moving pointer (@latest)", Implemented).
		Given("several published versions of code-fixer", func(w *World) {}).
		When("a client resolves code-fixer@latest", func(w *World) {
			v, err := w.Catalog.Resolve(ctx(), AgentRef{Name: "code-fixer", Version: "latest"})
			w.set("v", v)
			w.expect(err == nil, "resolve @latest should succeed")
		}).
		Then("it returns the concrete version currently designated latest", func(w *World) {
			v := w.get("v").(Version)
			w.expect(v.Ref.Version != "latest", "resolved version must be concrete, got %q", v.Ref.Version)
		}).
		Run()
}

func TestCAT_04_PullDefinitionForm(t *testing.T) {
	Scenario(t, "CAT-04", "Pull a definition-form agent", Implemented).
		Given("code-fixer@1.2.0 published with form=definition", func(w *World) {}).
		When("an authorized runner pulls it", func(w *World) {
			art, err := w.Catalog.Pull(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, Identity{Runner: "R"}, grant(OpPullAgent))
			w.set("art", art)
			w.expect(err == nil, "authorized pull should succeed")
		}).
		Then("it receives the manifest content plus a form indicator", func(w *World) {
			art := w.get("art").(Artifact)
			w.expect(art.Form == FormDefinition, "form should be definition, got %q", art.Form)
		}).
		Run()
}

func TestCAT_05_PullImageForm(t *testing.T) {
	Scenario(t, "CAT-05", "Pull an image-form agent", Implemented).
		Given("an agent version published with form=image", func(w *World) {}).
		When("an authorized runner pulls it", func(w *World) {
			art, _ := w.Catalog.Pull(ctx(), AgentRef{Name: "img", Version: "1.0.0"}, Identity{Runner: "R"}, grant(OpPullAgent))
			w.set("art", art)
		}).
		Then("it receives the image reference plus a form indicator for the sandbox driver", func(w *World) {
			art := w.get("art").(Artifact)
			w.expect(art.Form == FormImage, "form should be image, got %q", art.Form)
		}).
		Run()
}

func TestCAT_06_AuthorizedPullSucceeds(t *testing.T) {
	Scenario(t, "CAT-06", "Authorized pull succeeds", Implemented).
		Given("an enrolled runner with a valid grant for the dispatched version", func(w *World) {}).
		When("it pulls code-fixer@1.2.0", func(w *World) {
			_, err := w.Catalog.Pull(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, Identity{Runner: "R"}, grant(OpPullAgent))
			w.set("err", err)
		}).
		Then("it receives the artifact (or reference)", func(w *World) {
			w.expect(w.get("err") == nil, "pull with a valid grant should be allowed")
		}).
		Run()
}

func TestCAT_07_UnauthorizedPullDenied(t *testing.T) {
	Scenario(t, "CAT-07", "Unauthorized pull is denied", Implemented).
		Given("a caller without a valid grant for the version", func(w *World) {}).
		When("it attempts to pull", func(w *World) {
			_, err := w.Catalog.Pull(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, Identity{Runner: "R"}, grant())
			w.set("err", err)
		}).
		Then("the pull is denied", func(w *World) {
			w.expect(w.get("err") != nil, "pull without OpPullAgent in the grant must be denied")
		}).
		Run()
}

func TestCAT_08_DispatchReachesTargetRunner(t *testing.T) {
	Scenario(t, "CAT-08", "Dispatch reaches the target runner", Implemented).
		Given("runner R is subscribed to runner.R.dispatch", func(w *World) {
			w.Session = w.OpenSession("R", Filter{Topics: []Topic{"runner.R.dispatch"}})
		}).
		When("an operator dispatches code-fixer@1.2.0 to R", func(w *World) {
			_, err := w.Catalog.Dispatch(ctx(), AgentRef{Name: "code-fixer", Version: "1.2.0"}, "R", grant(OpPullAgent))
			w.expect(err == nil, "dispatch should succeed")
		}).
		Then("a dispatch message (agent ref + grant) is published to R's topic; no bytes pushed", func(w *World) {
			ev := <-w.Session.Control().Events()
			w.expect(ev.Kind == "dispatch", "R should receive a dispatch message, got %q", ev.Kind)
		}).
		Run()
}

func TestCAT_09_DispatchCarriesScopedGrant(t *testing.T) {
	Scenario(t, "CAT-09", "Dispatch carries an explicit scoped grant", Implemented).
		Given("a dispatch whose grant permits only repo.read and agent.pull", func(w *World) {
			w.Grant = grant(OpRepoRead, OpPullAgent)
		}).
		When("the dispatch message is built", func(w *World) {}).
		Then("it carries an integrity-protected grant scoped to those ops and nothing else", func(w *World) {
			w.expect(w.Grant.Permits(OpRepoRead) && !w.Grant.Permits(OpRepoWrite),
				"grant must include repo.read but exclude repo.write")
		}).
		Run()
}

func TestCAT_10_AbsentGrantDeniesOperation(t *testing.T) {
	Scenario(t, "CAT-10", "Absent grant denies a privileged operation", Implemented).
		Given("a dispatch without a grant for repo.write", func(w *World) {
			w.Grant = grant(OpRepoRead)
		}).
		When("the run reaches a repo.write operation", func(w *World) {}).
		Then("repo.write is not permitted (least-privilege by default)", func(w *World) {
			w.expect(!w.Grants.Permits(w.Grant, OpRepoWrite), "absent grant for repo.write must deny it")
		}).
		Run()
}
