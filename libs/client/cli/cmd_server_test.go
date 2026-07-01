package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// executeServerStart runs serverStartCmd with the given args, capturing output,
// and returns the captured output and any RunE error. A context is supplied
// because RunE uses cmd.Context().
func executeServerStart(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	serverStartCmd.SetOut(&out)
	serverStartCmd.SetErr(&out)
	serverStartCmd.SetArgs(args)
	serverStartCmd.SetContext(context.Background())
	err := serverStartCmd.RunE(serverStartCmd, args)
	return out.String(), err
}

// TestServerStart exercises the injection seam: with a fake starter wired via
// SetServerStarter, `server start --listen :9999` invokes it with ":9999"; with
// no starter wired, the command returns a clear "not available" error.
func TestServerStart(t *testing.T) {
	t.Run("invokes wired starter with listen override", func(t *testing.T) {
		var gotListen string
		called := false
		SetServerStarter(func(ctx context.Context, listen string) error {
			called = true
			gotListen = listen
			return nil
		})
		t.Cleanup(func() { SetServerStarter(nil) })

		if _, err := executeServerStart(t, "--listen", ":9999"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Fatal("expected wired starter to be called")
		}
		if gotListen != ":9999" {
			t.Errorf("listen = %q, want %q", gotListen, ":9999")
		}
	})

	t.Run("empty listen passed when flag omitted (no override)", func(t *testing.T) {
		gotListen := "sentinel"
		SetServerStarter(func(ctx context.Context, listen string) error {
			gotListen = listen
			return nil
		})
		t.Cleanup(func() { SetServerStarter(nil) })

		if _, err := executeServerStart(t); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotListen != "" {
			t.Errorf("listen = %q, want empty string (no override)", gotListen)
		}
	})

	t.Run("no starter wired returns clear error", func(t *testing.T) {
		SetServerStarter(nil)

		_, err := executeServerStart(t)
		if err == nil {
			t.Fatal("expected error when no starter is wired")
		}
		if !strings.Contains(err.Error(), "not available") {
			t.Errorf("error %q does not mention %q", err.Error(), "not available")
		}
	})
}
