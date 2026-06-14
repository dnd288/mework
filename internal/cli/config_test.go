package cli

import "testing"

func TestConfigSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	const prof = "dev"

	in := &Config{
		BaseURL:     "https://mello.example/api/v1",
		WorkspaceID: "ws1",
		Token:       "mello_pat_x",
		MCPURL:      "https://mcp.example",
	}
	in.Daemon.TriggerKeyword = "/go"
	in.Daemon.PollIntervalSeconds = 10
	if err := in.Save(prof); err != nil {
		t.Fatal(err)
	}

	out, err := LoadConfig(prof)
	if err != nil {
		t.Fatal(err)
	}
	if out.BaseURL != in.BaseURL || out.Token != in.Token || out.MCPURL != in.MCPURL {
		t.Errorf("round-trip mismatch: %+v", out)
	}
	if out.Daemon.TriggerKeyword != "/go" || out.Daemon.PollIntervalSeconds != 10 {
		t.Errorf("daemon config lost: %+v", out.Daemon)
	}
}

func TestLoadConfigMissingIsEmpty(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	cfg, err := LoadConfig("nonexistent")
	if err != nil {
		t.Fatalf("missing config should not error: %v", err)
	}
	if cfg.Token != "" {
		t.Error("expected empty config for missing file")
	}
}

func TestProfileIsolation(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	a := &Config{Token: "tok-a"}
	b := &Config{Token: "tok-b"}
	if err := a.Save("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := b.Save("beta"); err != nil {
		t.Fatal(err)
	}
	la, _ := LoadConfig("alpha")
	lb, _ := LoadConfig("beta")
	if la.Token != "tok-a" || lb.Token != "tok-b" {
		t.Errorf("profiles leaked: alpha=%q beta=%q", la.Token, lb.Token)
	}
}
