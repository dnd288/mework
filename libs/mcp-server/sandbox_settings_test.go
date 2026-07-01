package main

import (
	"encoding/json"
	"testing"
)


// TestGenerateSandboxSettings exercises the .claude/settings.json template
// generator. It validates the output structure, field values, and JSON
// round-trip consistency.
//
// RED step: fails because GenerateSandboxSettings is a stub that returns
// empty/nil values, not the JSON document the tests assert on.
func TestGenerateSandboxSettings(t *testing.T) {
	t.Run("returns valid JSON with mework-mcp", func(t *testing.T) {
		result, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: "mework-mcp",
		})
		if err != nil {
			t.Fatalf("GenerateSandboxSettings: %v", err)
		}
		if result == "" {
			t.Fatal("expected non-empty JSON output")
		}

		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(result), &doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		servers, ok := doc["mcpServers"].(map[string]interface{})
		if !ok {
			t.Fatal("expected mcpServers object in output")
		}

		entry, ok := servers["mework"].(map[string]interface{})
		if !ok {
			t.Fatal("expected mework entry in mcpServers")
		}
		if entry == nil {
			t.Fatal("mework entry is nil")
		}
	})

	t.Run("mework entry has correct structure", func(t *testing.T) {
		result, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: "mework-mcp",
		})
		if err != nil {
			t.Fatalf("GenerateSandboxSettings: %v", err)
		}

		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(result), &doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		servers := doc["mcpServers"].(map[string]interface{})
		entry := servers["mework"].(map[string]interface{})

		cmd, ok := entry["command"].(string)
		if !ok || cmd != "mework-mcp" {
			t.Errorf("command = %q, want %q", cmd, "mework-mcp")
		}

		args, ok := entry["args"].([]interface{})
		if !ok {
			t.Fatal("expected args array")
		}
		if len(args) != 0 {
			t.Errorf("expected empty args, got %v", args)
		}

		env, ok := entry["env"].(map[string]interface{})
		if !ok {
			t.Fatal("expected env object")
		}
		sessionID, ok := env["MEWORK_SESSION_ID"].(string)
		if !ok || sessionID != "auto" {
			t.Errorf("MEWORK_SESSION_ID = %q, want %q", sessionID, "auto")
		}
	})

	t.Run("withGitHub=true adds github MCP server", func(t *testing.T) {
		result, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: "mework-mcp",
			WithGitHub:       true,
		})
		if err != nil {
			t.Fatalf("GenerateSandboxSettings: %v", err)
		}

		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(result), &doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		servers := doc["mcpServers"].(map[string]interface{})

		ghEntry, ok := servers["github"].(map[string]interface{})
		if !ok {
			t.Fatal("expected github entry in mcpServers")
		}
		cmd, ok := ghEntry["command"].(string)
		if !ok || cmd != "gh" {
			t.Errorf("github command = %q, want %q", cmd, "gh")
		}
		args, ok := ghEntry["args"].([]interface{})
		if !ok || len(args) != 1 || args[0] != "mcp" {
			t.Errorf("github args = %v, want [\"mcp\"]", args)
		}
	})

	t.Run("withGitHub=false omits github entry", func(t *testing.T) {
		result, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: "mework-mcp",
			WithGitHub:       false,
		})
		if err != nil {
			t.Fatalf("GenerateSandboxSettings: %v", err)
		}

		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(result), &doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		servers := doc["mcpServers"].(map[string]interface{})

		if _, ok := servers["github"]; ok {
			t.Error("unexpected github entry in mcpServers when WithGitHub=false")
		}
	})

	t.Run("custom binary path works", func(t *testing.T) {
		customPath := "/usr/local/bin/mework-mcp"
		result, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: customPath,
		})
		if err != nil {
			t.Fatalf("GenerateSandboxSettings: %v", err)
		}

		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(result), &doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		servers := doc["mcpServers"].(map[string]interface{})
		entry := servers["mework"].(map[string]interface{})
		cmd, ok := entry["command"].(string)
		if !ok || cmd != customPath {
			t.Errorf("command = %q, want %q", cmd, customPath)
		}
	})

	t.Run("output is valid .claude/settings.json", func(t *testing.T) {
		result, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: "mework-mcp",
			WithGitHub:       true,
		})
		if err != nil {
			t.Fatalf("GenerateSandboxSettings: %v", err)
		}

		// Unmarshal into a generic map (Claude's config loader parses JSON).
		var doc map[string]interface{}
		if err := json.Unmarshal([]byte(result), &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Verify the top-level structure has mcpServers.
		servers, ok := doc["mcpServers"].(map[string]interface{})
		if !ok {
			t.Fatal("document missing mcpServers")
		}

		// Verify each server entry has the required MCP fields.
		for name, entry := range servers {
			m, ok := entry.(map[string]interface{})
			if !ok {
				t.Errorf("server %q entry is not an object", name)
				continue
			}
			if _, ok := m["command"]; !ok {
				t.Errorf("server %q missing command", name)
			}
		}

		// Round-trip: re-marshal and re-unmarshal.
		reJSON, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		var reloaded map[string]interface{}
		if err := json.Unmarshal(reJSON, &reloaded); err != nil {
			t.Fatalf("round-trip unmarshal: %v", err)
		}
		if _, ok := reloaded["mcpServers"]; !ok {
			t.Error("round-tripped document missing mcpServers")
		}
	})

	t.Run("empty command returns error", func(t *testing.T) {
		_, err := GenerateSandboxSettings(SandboxSettingsInput{
			MeworkMCPCommand: "",
		})
		if err == nil {
			t.Error("expected error for empty meworkMCPCommand, got nil")
		}
	})
}
