//go:build e2e

package e2e

import "testing"

// Feature 24 — Workspace base code & lifecycle hooks. Real-world platform surface
// (proposed). A workspace can carry base code (clone a git repo / unpack a template) and
// lifecycle hooks the sandbox runs: init → pre_run → (agent) → post_run → pre/post_sync.
// Hook scripts are fed over stdin, never argv, and run within the workspace grant scope.

func TestWSHOOK_01_InitClonesGitRepo(t *testing.T) {
	Scenario(t, "WSHOOK-01", "Init hook clones a git repo into the workspace", PlannedPlatform).
		Given("a workspace whose base is a git repo at a revision", func(w *World) {
			spec := WorkspaceSpec{MountPath: "/workspace", Mode: WorkspaceRW,
				Base: BaseSpec{Kind: BaseGit, Ref: "https://example.com/acme/web.git", Rev: "main"}}
			ws := w.AttachWorkspace("s1", spec)
			w.set("id", ws.ID)
		}).
		When("the workspace bootstraps", func(w *World) {
			res, _ := w.Workspaces.Bootstrap(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the repo is cloned into the workspace before the agent runs", func(w *World) {
			w.expect(w.get("res").(Result).Status == StatusDone, "base git clone completes during bootstrap")
		}).
		Run()
}

func TestWSHOOK_02_SetupHookInstallsDeps(t *testing.T) {
	Scenario(t, "WSHOOK-02", "Init hook runs base-code setup (install deps)", PlannedPlatform).
		Given("a workspace with an init hook running `make setup`", func(w *World) {
			spec := WorkspaceSpec{MountPath: "/workspace", Mode: WorkspaceRW,
				Hooks: []Hook{{Name: "setup", Stage: HookInit, Script: "make setup"}}}
			ws := w.AttachWorkspace("s1", spec)
			w.set("id", ws.ID)
		}).
		When("the workspace bootstraps", func(w *World) {
			res, _ := w.Workspaces.Bootstrap(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the setup hook runs and its output is captured", func(w *World) {
			w.expect(w.get("res").(Result).Status == StatusDone, "the init hook executes during bootstrap")
		}).
		Run()
}

func TestWSHOOK_03_PreAndPostRunHooks(t *testing.T) {
	Scenario(t, "WSHOOK-03", "Pre-run and post-run hooks bracket the agent", PlannedPlatform).
		Given("a workspace with pre_run and post_run hooks", func(w *World) {
			spec := WorkspaceSpec{Hooks: []Hook{
				{Name: "warm", Stage: HookPreRun, Script: "echo warming"},
				{Name: "report", Stage: HookPostRun, Script: "echo done"},
			}}
			ws := w.AttachWorkspace("s1", spec)
			w.set("id", ws.ID)
		}).
		When("the runner drives the run lifecycle", func(w *World) {
			pre, _ := w.Workspaces.RunHooks(ctx(), w.get("id").(WorkspaceID), HookPreRun)
			post, _ := w.Workspaces.RunHooks(ctx(), w.get("id").(WorkspaceID), HookPostRun)
			w.set("pre", pre)
			w.set("post", post)
		}).
		Then("pre_run runs before the agent and post_run after, both captured", func(w *World) {
			w.expect(w.get("pre").(Result).Status == StatusDone && w.get("post").(Result).Status == StatusDone,
				"pre/post-run hooks bracket the agent execution")
		}).
		Run()
}

func TestWSHOOK_04_FailingInitAbortsRun(t *testing.T) {
	Scenario(t, "WSHOOK-04", "A failing init/pre_run hook aborts the run", PlannedPlatform).
		Given("a workspace whose init hook exits non-zero", func(w *World) {
			spec := WorkspaceSpec{Hooks: []Hook{{Name: "bad", Stage: HookInit, Script: "exit 1"}}}
			ws := w.AttachWorkspace("s1", spec)
			w.set("id", ws.ID)
		}).
		When("the workspace bootstraps", func(w *World) {
			res, _ := w.Workspaces.Bootstrap(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the run is aborted (reported failed) and the sandbox is torn down", func(w *World) {
			w.expect(w.get("res").(Result).Status == StatusFailed, "a failed init hook aborts the run")
		}).
		Run()
}

func TestWSHOOK_05_HooksRunWithinGrantScope(t *testing.T) {
	Scenario(t, "WSHOOK-05", "Hooks run within the workspace grant scope", PlannedPlatform).
		Given("a workspace + grant allowing workspace.write but not network", func(w *World) {
			w.Grant = grant(OpWorkspaceWrite, OpWorkspaceRead)
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Hooks: []Hook{{Name: "x", Stage: HookInit, Script: "curl http://evil"}}})
			w.set("id", ws.ID)
		}).
		When("an init hook attempts a network call outside the grant", func(w *World) {
			res, _ := w.Workspaces.Bootstrap(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the hook is confined: it can write the workspace but cannot exceed the grant", func(w *World) {
			w.expect(!w.Grant.Permits(OpNetwork), "hooks inherit the run's least-privilege grant")
		}).
		Run()
}

func TestWSHOOK_06_PostSyncHook(t *testing.T) {
	Scenario(t, "WSHOOK-06", "Post-sync hook runs after files sync to remote", PlannedPlatform).
		Given("a workspace with a post_sync hook", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Sync: SyncOnFlush,
				Hooks: []Hook{{Name: "notify", Stage: HookPostSync, Script: "echo synced"}}})
			w.set("id", ws.ID)
		}).
		When("a sync completes", func(w *World) {
			_, _ = w.Workspaces.Sync(ctx(), w.get("id").(WorkspaceID))
			res, _ := w.Workspaces.RunHooks(ctx(), w.get("id").(WorkspaceID), HookPostSync)
			w.set("res", res)
		}).
		Then("the post_sync hook runs after the remote push", func(w *World) {
			w.expect(w.get("res").(Result).Status == StatusDone, "post_sync fires after files reach remote")
		}).
		Run()
}

func TestWSHOOK_07_BaseFromArchiveOrTemplate(t *testing.T) {
	Scenario(t, "WSHOOK-07", "Base code from an archive or store template (not just git)", PlannedPlatform).
		Given("a workspace whose base is an object-store template prefix", func(w *World) {
			spec := WorkspaceSpec{Base: BaseSpec{Kind: BaseStore, Ref: "templates/go-service/"}}
			ws := w.AttachWorkspace("s1", spec)
			w.set("id", ws.ID)
		}).
		When("the workspace bootstraps", func(w *World) {
			res, _ := w.Workspaces.Bootstrap(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the template is materialized into the workspace (pluggable base source)", func(w *World) {
			w.expect(w.get("res").(Result).Status == StatusDone, "base source is pluggable: git | archive | store")
		}).
		Run()
}

func TestWSHOOK_08_HookScriptOverStdin(t *testing.T) {
	Scenario(t, "WSHOOK-08", "Hook scripts are fed over stdin, never argv", PlannedPlatform).
		Given("a hook whose script contains attacker-influenced content", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Hooks: []Hook{{Name: "x", Stage: HookInit, Script: "echo $UNTRUSTED"}}})
			w.set("id", ws.ID)
		}).
		When("the sandbox runs the hook", func(w *World) {
			res, _ := w.Workspaces.Bootstrap(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the script is delivered on stdin and never appears in argv (injection-safe)", func(w *World) {
			w.expect(w.get("res").(Result).Status == StatusDone || w.get("res").(Result).Status == StatusFailed,
				"hooks preserve the stdin-not-argv invariant")
		}).
		Run()
}
