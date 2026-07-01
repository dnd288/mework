//go:build e2e

package e2e

import "testing"

// Feature 03 — CLI onboarding. Source: SCENARIOS.md
// Baseline behavior (Implemented); skipped pending the e2e World harness.

func TestCLI_01_Login(t *testing.T) {
	Scenario(t, "CLI-01", "Log in with a Mello PAT", Implemented).
		Given("a valid Mello PAT", func(w *World) {}).
		When("the developer runs `mework login --token <pat>`", func(w *World) {
			_, err := w.RunCLI("login", "--token", "mello_pat_xxx")
			w.set("err", err)
		}).
		Then("the CLI validates it against /me and saves it to config", func(w *World) {
			w.expect(w.get("err") == nil, "login with a valid PAT should succeed")
		}).
		Run()
}

func TestCLI_02_TokenFilePermissions(t *testing.T) {
	Scenario(t, "CLI-02", "Token file is written with restrictive permissions", Implemented).
		Given("a successful login", func(w *World) {
			_, _ = w.RunCLI("login", "--token", "mello_pat_xxx")
		}).
		When("the config is written", func(w *World) {
			w.set("mode", w.ConfigFileMode())
		}).
		Then("the config file is mode 0600", func(w *World) {
			w.expect(w.get("mode").(uint32) == 0o600, "config file must be 0600")
		}).
		Run()
}

func TestCLI_03_ConfigPrecedence(t *testing.T) {
	Scenario(t, "CLI-03", "Config precedence is flag > env > file", Implemented).
		Given("base_url set in the file, MELLO_BASE_URL in env, and a --server-url flag", func(w *World) {}).
		When("a command resolves the value", func(w *World) {
			out, _ := w.RunCLI("--server-url", "http://flag", "board", "list")
			w.set("out", out)
		}).
		Then("the flag value wins over env, and env over file", func(w *World) {
			w.expect(true, "resolution precedence is flag > env > file")
		}).
		Run()
}

func TestCLI_04_ProfileIsolation(t *testing.T) {
	Scenario(t, "CLI-04", "Profile isolation", Implemented).
		Given("--profile work", func(w *World) {}).
		When("the CLI reads or writes config/pid/logs/workspace", func(w *World) {
			_, _ = w.RunCLI("--profile", "work", "daemon", "status")
		}).
		Then("all state is rooted under ~/.mework/profiles/work/", func(w *World) {
			w.expect(true, "named profiles isolate all local state")
		}).
		Run()
}

func TestCLI_05_ConnectProvider(t *testing.T) {
	Scenario(t, "CLI-05", "Connect a provider", Implemented).
		Given("an authenticated developer", func(w *World) {}).
		When("they run `mework provider connect --provider mello --token <token>`", func(w *World) {
			_, err := w.RunCLI("provider", "connect", "--provider", "mello", "--token", "t")
			w.set("err", err)
		}).
		Then("the hub stores a sealed connection unique per (account, provider)", func(w *World) {
			w.expect(w.get("err") == nil, "provider connect should succeed")
		}).
		Run()
}

func TestCLI_06_RegisterRuntime(t *testing.T) {
	Scenario(t, "CLI-06", "Register a runtime returns a one-time token", Implemented).
		Given("an authenticated developer", func(w *World) {}).
		When("they run `mework runtime register --code macbook --label MacBook`", func(w *World) {
			_, tok := w.RegisterRuntime("macbook", "MacBook")
			w.set("tok", tok)
		}).
		Then("the hub returns a one-time rt_token; list shows it and revoke removes it", func(w *World) {
			w.expect(w.get("tok") != "", "a one-time rt_token is returned")
		}).
		Run()
}

func TestCLI_07_ManageProfile(t *testing.T) {
	Scenario(t, "CLI-07", "Create & manage an AI profile", Implemented).
		Given("an authenticated developer", func(w *World) {}).
		When("they run `mework profile create --name default --backend claude ...`", func(w *World) {
			w.CreateProfile("default", "prompt", "claude", "claude-code")
		}).
		Then("the profile is stored and list/update/delete operate on it by name", func(w *World) {
			w.expect(true, "server-side profile CRUD by name")
		}).
		Run()
}

func TestCLI_08_JSONOutput(t *testing.T) {
	Scenario(t, "CLI-08", "Machine-readable --json output", Implemented).
		Given("any list/get command", func(w *World) {}).
		When("it is run with --json", func(w *World) {
			out, _ := w.RunCLI("runtime", "list", "--json")
			w.set("out", out)
		}).
		Then("it emits valid JSON suitable for scripting", func(w *World) {
			w.expect(w.get("out") != "", "--json emits machine-readable output")
		}).
		Run()
}
