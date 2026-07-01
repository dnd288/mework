package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// findSub returns the immediate subcommand of parent whose first Use word equals
// name, or nil.
func findSub(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestWorkspaceSubcommandsRegistered asserts pack/push/pull are wired onto the
// workspace command tree (realizes "mework workspace pack|push|pull").
func TestWorkspaceSubcommandsRegistered(t *testing.T) {
	for _, name := range []string{"pack", "push", "pull"} {
		t.Run(name, func(t *testing.T) {
			if findSub(workspaceCmd, name) == nil {
				t.Errorf("expected workspace subcommand %q to be registered", name)
			}
		})
	}
}

// TestWorkspacePackArgValidation asserts `workspace pack` requires a directory
// argument — invoking it with no args produces a usage/arg error.
func TestWorkspacePackArgValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "missing dir is an error", args: []string{}, wantErr: true},
		{name: "one dir arg is accepted", args: []string{"some-dir"}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if workspacePackCmd.Args == nil {
				t.Fatal("workspacePackCmd.Args validator is not set")
			}
			err := workspacePackCmd.Args(workspacePackCmd, tt.args)
			if tt.wantErr && err == nil {
				t.Errorf("args %v: expected error, got nil", tt.args)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("args %v: unexpected error: %v", tt.args, err)
			}
		})
	}
}
