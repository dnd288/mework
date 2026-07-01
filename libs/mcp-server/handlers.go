// Package main implements the mework MCP server binary.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolHandler is the simplified handler signature used by ToolRegistry.
// It receives parsed arguments and returns a result or an error.
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// toolEntry stores a registered tool definition and its handler function.
type toolEntry struct {
	tool    mcp.Tool
	handler ToolHandler
}

// ToolRegistry manages a mapping of tool names to their handlers.
type ToolRegistry struct {
	entries map[string]toolEntry
}

// NewToolRegistry creates a new empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		entries: make(map[string]toolEntry),
	}
}

// Register adds a tool and its handler to the registry under the given name.
func (r *ToolRegistry) Register(name string, tool mcp.Tool, handler ToolHandler) {
	r.entries[name] = toolEntry{
		tool:    tool,
		handler: handler,
	}
}

// ServerTools returns the registered tools as mcp-go server.ServerTool values
// suitable for use with server.NewMCPServer or mcptest.NewServer.
func (r *ToolRegistry) ServerTools() []server.ServerTool {
	var out []server.ServerTool
	for _, e := range r.entries {
		h := e.handler
		out = append(out, server.ServerTool{
			Tool: e.tool,
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args, _ := req.Params.Arguments.(map[string]interface{})
				result, err := h(ctx, args)
				if err != nil {
					return nil, err
				}
				content, convErr := toContent(result)
				if convErr != nil {
					return nil, convErr
				}
				return &mcp.CallToolResult{Content: content}, nil
			},
		})
	}
	return out
}

// toContent converts a handler result value to MCP Content for a CallToolResult.
func toContent(v interface{}) ([]mcp.Content, error) {
	switch val := v.(type) {
	case string:
		return []mcp.Content{mcp.NewTextContent(val)}, nil
	case []mcp.Content:
		return val, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("mcp-server: marshal result: %w", err)
		}
		return []mcp.Content{mcp.NewTextContent(string(b))}, nil
	}
}
