package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mework/internal/cli"
	"mework/internal/mello"
)

// newRESTClient builds a Mello REST client from resolved config + flags/env.
// Returns an error when no token is available so commands fail clearly.
func newRESTClient(cmd *cobra.Command) (*mello.Client, *cli.Config, error) {
	cfg, err := cli.LoadConfig(profile())
	if err != nil {
		return nil, nil, err
	}
	token := cli.ResolveToken(cfg)
	if token == "" {
		return nil, nil, fmt.Errorf("not authenticated — run `mello login --token <mello_pat_...>` or set MELLO_API_KEY")
	}
	baseURL := cli.ResolveBaseURL(cmd, cfg)
	return mello.NewClient(baseURL, token, 30*time.Second, version), cfg, nil
}

// requireWorkspaceID resolves the workspace id or errors if unset.
func requireWorkspaceID(cmd *cobra.Command, cfg *cli.Config) (string, error) {
	ws := cli.ResolveWorkspaceID(cmd, cfg)
	if ws == "" {
		return "", fmt.Errorf("workspace id required — pass --workspace-id, set MELLO_WORKSPACE_ID, or `mello config set workspace_id <id>`")
	}
	return ws, nil
}
