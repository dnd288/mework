package cli

import "testing"

func TestMaskToken(t *testing.T) {
	cases := map[string]string{
		"":                     "",
		"abc":                  "****",
		"mello_pat_secret1234": "****1234",
	}
	for in, want := range cases {
		if got := maskToken(in); got != want {
			t.Errorf("maskToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigKeysWhitelist(t *testing.T) {
	// Known keys must be present; an unknown key must be absent.
	for _, key := range []string{"base_url", "workspace_id", "server_url", "rt_token", "daemon.trigger_keyword", "daemon.done_column_id"} {
		if _, ok := configKeys[key]; !ok {
			t.Errorf("expected %q in configKeys whitelist", key)
		}
	}
	if _, ok := configKeys["token"]; ok {
		t.Error("token must NOT be settable via config set")
	}
}

func TestRootCmdName(t *testing.T) {
	if rootCmd.Use != "mework" {
		t.Errorf("expected rootCmd.Use to be %q, got %q", "mework", rootCmd.Use)
	}
}
