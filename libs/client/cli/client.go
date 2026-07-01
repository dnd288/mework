package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"mework/libs/shared/config"
	"mework/libs/shared/providers/mello"
	"mework/libs/client/subscribe"
)

// newRESTClient builds a Mello REST client from resolved config + flags/env.
// Returns an error when no token is available so commands fail clearly.
func newRESTClient(cmd *cobra.Command) (*mello.Client, *config.Config, error) {
	cfg, err := config.LoadConfig(profile())
	if err != nil {
		return nil, nil, err
	}
	token := ResolveToken(cfg)
	if token == "" {
		return nil, nil, fmt.Errorf("not authenticated — run `mework runner enroll` or set MEWORK_API_KEY / MELLO_API_KEY")
	}
	baseURL := ResolveBaseURL(cmd, cfg)
	return mello.NewClient(baseURL, token, 30*time.Second, version), cfg, nil
}

// newMeworkClient builds a client for the mework-server.
func newMeworkClient() (*subscribe.Client, *config.Config, error) {
	cfg, err := config.LoadConfig(profile())
	if err != nil {
		return nil, nil, err
	}
	serverURL := cfg.ServerURL
	if serverURL == "" {
		serverURL = os.Getenv("MEWORK_SERVER_URL")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080" // Default fallback
	}
	return subscribe.NewClient(serverURL, 10*time.Second), cfg, nil
}

// requireWorkspaceID resolves the workspace id or errors if unset.
func requireWorkspaceID(cmd *cobra.Command, cfg *config.Config) (string, error) {
	ws := ResolveWorkspaceID(cmd, cfg)
	if ws == "" {
		return "", fmt.Errorf("workspace id required — pass --workspace-id or set MEWORK_WORKSPACE_ID / MELLO_WORKSPACE_ID")
	}
	return ws, nil
}
