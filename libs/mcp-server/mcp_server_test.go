package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

// TestMCPServerScaffold covers the foundation scenarios from the delta spec:
//   - ListTools returns registered tools
//   - CallTool with known tool calls the handler
//   - CallTool with unknown tool returns error (not a panic)
//   - Ping returns pong
//
// Red step: these tests reference NewToolRegistry, ToolRegistry.Register,
// and ToolRegistry.ServerTools which are not yet implemented.
func TestMCPServerScaffold(t *testing.T) {
	t.Run("ListTools returns registered tools", func(t *testing.T) {
		reg := NewToolRegistry()
		reg.Register("spawn_sandbox",
			mcp.NewTool("spawn_sandbox",
				mcp.WithDescription("Spawns a child sandbox"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt for the sandbox agent")),
			),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]string{"sandbox_id": "sb-123"}, nil
			},
		)
		reg.Register("get_sandbox_status",
			mcp.NewTool("get_sandbox_status",
				mcp.WithDescription("Gets the status of a child sandbox"),
				mcp.WithString("sandbox_id", mcp.Required(), mcp.Description("ID of the sandbox")),
			),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return map[string]string{"status": "running"}, nil
			},
		)

		tools := reg.ServerTools()

		srv, err := mcptest.NewServer(t, tools...)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer srv.Close()

		result, err := srv.Client().ListTools(t.Context(), mcp.ListToolsRequest{})
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}

		if len(result.Tools) == 0 {
			t.Fatal("expected at least one tool")
		}

		names := make(map[string]bool)
		for _, tool := range result.Tools {
			names[tool.Name] = true
		}
		if !names["spawn_sandbox"] {
			t.Error("expected spawn_sandbox in tool list")
		}
		if !names["get_sandbox_status"] {
			t.Error("expected get_sandbox_status in tool list")
		}
	})

	t.Run("CallTool with known tool calls the handler", func(t *testing.T) {
		reg := NewToolRegistry()
		reg.Register("echo",
			mcp.NewTool("echo",
				mcp.WithDescription("Echoes back the message"),
				mcp.WithString("message", mcp.Required(), mcp.Description("Message to echo")),
			),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				msg, _ := args["message"].(string)
				return msg, nil
			},
		)

		tools := reg.ServerTools()

		srv, err := mcptest.NewServer(t, tools...)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer srv.Close()

		result, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "echo",
				Arguments: map[string]interface{}{
					"message": "hello world",
				},
			},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}
		if len(result.Content) == 0 {
			t.Fatal("expected non-empty content")
		}
		tc, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}
		if tc.Text != "hello world" {
			t.Errorf("content text = %q, want %q", tc.Text, "hello world")
		}
	})

	t.Run("CallTool with unknown tool returns error", func(t *testing.T) {
		reg := NewToolRegistry()
		reg.Register("known_tool",
			mcp.NewTool("known_tool", mcp.WithDescription("A known tool")),
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return "ok", nil
			},
		)

		tools := reg.ServerTools()

		srv, err := mcptest.NewServer(t, tools...)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer srv.Close()

		_, err = srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "nonexistent_tool",
				Arguments: map[string]interface{}{},
			},
		})
		if err == nil {
			t.Error("expected error for unknown tool, got nil")
		}
	})

	t.Run("Ping returns pong", func(t *testing.T) {
		reg := NewToolRegistry()
		tools := reg.ServerTools()

		srv, err := mcptest.NewServer(t, tools...)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer srv.Close()

		err = srv.Client().Ping(t.Context())
		if err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})
}
