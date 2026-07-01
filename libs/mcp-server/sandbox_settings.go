package main

import (
	"encoding/json"
	"fmt"
)

// SandboxSettingsInput defines the input parameters for GenerateSandboxSettings.
type SandboxSettingsInput struct {
	// MeworkMCPCommand is the path or name of the mework-mcp binary.
	// Required: must be non-empty.
	MeworkMCPCommand string

	// WithGitHub controls whether a gh mcp entry is included in the output.
	// Optional: defaults to false.
	WithGitHub bool

	// AdditionalEnv is an optional set of extra environment variables to
	// include in the mework-mcp server entry.
	AdditionalEnv map[string]string
}

// GenerateSandboxSettings produces a .claude/settings.json document as a JSON
// string, configuring the mework-mcp stdio MCP server (and optionally the gh
// MCP server) for an orchestrator sandbox.
func GenerateSandboxSettings(input SandboxSettingsInput) (string, error) {
	if input.MeworkMCPCommand == "" {
		return "", fmt.Errorf("meworkMCPCommand must be non-empty")
	}

	env := map[string]interface{}{
		"MEWORK_SESSION_ID": "auto",
	}
	for k, v := range input.AdditionalEnv {
		env[k] = v
	}

	mcpServers := map[string]interface{}{
		"mework": map[string]interface{}{
			"command": input.MeworkMCPCommand,
			"args":    []interface{}{},
			"env":     env,
		},
	}

	if input.WithGitHub {
		mcpServers["github"] = map[string]interface{}{
			"command": "gh",
			"args":    []interface{}{"mcp"},
		}
	}

	doc := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	result, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal settings: %w", err)
	}

	return string(result), nil
}
