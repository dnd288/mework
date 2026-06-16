package cli

import (
	"encoding/json"
	"os"
)

// DefaultBaseURL is the Mello REST API base when not overridden.
const DefaultBaseURL = "https://mello.mezon.vn/api/v1"

// DaemonConfig holds the agent-runtime settings persisted in config.json.
type DaemonConfig struct {
	// TriggerKeyword fires the agent when found in a ticket comment (default "/run").
	TriggerKeyword string `json:"trigger_keyword,omitempty"`
	// PollIntervalSeconds is the board poll cadence (default 5).
	PollIntervalSeconds int `json:"poll_interval_seconds,omitempty"`
	// WatchBoardIDs limits polling to specific boards; empty means all accessible boards.
	WatchBoardIDs []string `json:"watch_board_ids,omitempty"`
	// DoneColumnID, when set, moves a ticket here after the agent finishes.
	DoneColumnID string `json:"done_column_id,omitempty"`
	// Backends lists the AI CLIs to detect/use (e.g. claude, codex, opencode).
	Backends []string `json:"backends,omitempty"`
}

// Config is the on-disk CLI/daemon configuration for a profile.
type Config struct {
	BaseURL      string       `json:"base_url,omitempty"`
	WorkspaceID  string       `json:"workspace_id,omitempty"`
	Token        string       `json:"token,omitempty"`
	ServerURL    string       `json:"server_url,omitempty"`
	RuntimeToken string       `json:"rt_token,omitempty"`
	Daemon       DaemonConfig `json:"daemon,omitempty"`
}

// LoadConfig reads the profile config from disk. A missing file yields a
// zero-value Config (not an error) so first-run commands work.
func LoadConfig(profile string) (*Config, error) {
	path := ConfigPath(profile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to the profile path with private permissions,
// creating the profile directory as needed.
func (c *Config) Save(profile string) error {
	if err := ensureDir(ProfileDir(profile)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// 0600: the file holds the bearer token.
	return os.WriteFile(ConfigPath(profile), data, 0o600)
}
