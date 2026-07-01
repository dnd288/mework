package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

// setupSessionTools creates a ToolRegistry with the session context tools
// registered, wrapped in an mcptest.Server. Env vars should be set before
// calling because the handler reads them at call time.
//
// RED step: fails because NewSessionHandler (defined in session.go) is not
// yet implemented. The test declares the expected API surface.
func setupSessionTools(t *testing.T) *mcptest.Server {
	t.Helper()

	handler := NewSessionHandler() // UNDEFINED SYMBOL — RED failure
	if handler == nil {
		t.Fatal("NewSessionHandler returned nil")
	}

	reg := NewToolRegistry()

	reg.Register("get_session_context",
		mcp.NewTool("get_session_context",
			mcp.WithDescription("Returns session context identity and workspace info"),
		),
		handler.GetSessionContext, // UNDEFINED SYMBOL
	)

	reg.Register("write_artifact",
		mcp.NewTool("write_artifact",
			mcp.WithDescription("Writes content to a file in the session workspace"),
			mcp.WithString("path", mcp.Required(),
				mcp.Description("Relative path within the workspace")),
			mcp.WithString("content", mcp.Required(),
				mcp.Description("Content to write")),
			mcp.WithString("encoding",
				mcp.Description("Encoding: 'text' (default) or 'base64'")),
		),
		handler.WriteArtifact, // UNDEFINED SYMBOL
	)

	tools := reg.ServerTools()
	srv, err := mcptest.NewServer(t, tools...)
	if err != nil {
		t.Fatalf("mcptest.NewServer: %v", err)
	}
	return srv
}

// TestSessionTools exercises the session context MCP tools:
//   - get_session_context
//   - write_artifact
//
// RED step: fails because NewSessionHandler (in session.go) is not yet
// implemented. The test declares the expected API surface and scenarios
// drawn from the delta-spec requirements.
func TestSessionTools(t *testing.T) {
	// -----------------------------------------------------------------------
	// get_session_context
	// -----------------------------------------------------------------------
	t.Run("get_session_context", func(t *testing.T) {
		tests := []struct {
			name string
			env  map[string]string
			want map[string]string
		}{
			{
				name: "returns all fields",
				env: map[string]string{
					"MEWORK_SESSION_ID":          "session-42",
					"MEWORK_SESSION_OWNER":       "alice",
					"MEWORK_SESSION_TENANT":      "acme-corp",
					"MEWORK_WORKSPACE_PATH":      "/workspaces/project-x",
					"MEWORK_PROVIDER":            "mello",
					"MEWORK_PROVIDER_RESOURCE":   "ticket-123",
				},
				want: map[string]string{
					"session_id":        "session-42",
					"owner":             "alice",
					"tenant":            "acme-corp",
					"workspace_path":    "/workspaces/project-x",
					"provider":          "mello",
					"provider_resource": "ticket-123",
				},
			},
			{
				name: "returns defaults for unset fields",
				env:  map[string]string{}, // all vars cleared
				want: map[string]string{
					"session_id":        "",
					"owner":             "",
					"tenant":            "",
					"workspace_path":    "",
					"provider":          "",
					"provider_resource": "",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Clear all session-related env vars first.
				os.Unsetenv("MEWORK_SESSION_ID")
				os.Unsetenv("MEWORK_SESSION_OWNER")
				os.Unsetenv("MEWORK_SESSION_TENANT")
				os.Unsetenv("MEWORK_WORKSPACE_PATH")
				os.Unsetenv("MEWORK_PROVIDER")
				os.Unsetenv("MEWORK_PROVIDER_RESOURCE")

				// Set the env vars for this case.
				for k, v := range tt.env {
					t.Setenv(k, v)
				}

				srv := setupSessionTools(t)
				defer srv.Close()

				result, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
					Params: mcp.CallToolParams{
						Name:      "get_session_context",
						Arguments: map[string]interface{}{},
					},
				})
				if err != nil {
					t.Fatalf("CallTool: %v", err)
				}
				if result.IsError {
					t.Fatal("expected success, got isError=true")
				}

				m := parseResult(t, result)

				for key, want := range tt.want {
					got, ok := m[key].(string)
					if !ok {
						t.Errorf("expected string field %q, got %T", key, m[key])
						continue
					}
					if got != want {
						t.Errorf("field %q = %q, want %q", key, got, want)
					}
				}
			})
		}
	})

	// -----------------------------------------------------------------------
	// write_artifact
	// -----------------------------------------------------------------------
	t.Run("write_artifact", func(t *testing.T) {
		tests := []struct {
			name      string
			workspace string // empty means unset
			path      string
			content   string
			encoding  string
			wantErr   bool
			checkFn   func(t *testing.T, wsDir string)
		}{
			{
				name:      "writes to workspace",
				workspace: t.TempDir(),
				path:      "test.txt",
				content:   "hello",
				encoding:  "",
				wantErr:   false,
				checkFn: func(t *testing.T, wsDir string) {
					absPath := filepath.Join(wsDir, "test.txt")
					data, err := os.ReadFile(absPath)
					if err != nil {
						t.Fatalf("ReadFile: %v", err)
					}
					if string(data) != "hello" {
						t.Errorf("content = %q, want %q", string(data), "hello")
					}
				},
			},
			{
				name:      "creates parent dirs",
				workspace: t.TempDir(),
				path:      "sub/dir/file.txt",
				content:   "content",
				encoding:  "",
				wantErr:   false,
				checkFn: func(t *testing.T, wsDir string) {
					absPath := filepath.Join(wsDir, "sub/dir/file.txt")
					data, err := os.ReadFile(absPath)
					if err != nil {
						t.Fatalf("ReadFile: %v", err)
					}
					if string(data) != "content" {
						t.Errorf("content = %q, want %q", string(data), "content")
					}
				},
			},
			{
				name:      "without workspace returns error",
				workspace: "", // unset
				path:      "test.txt",
				content:   "hello",
				encoding:  "",
				wantErr:   true,
				checkFn:   nil,
			},
			{
				name:      "with binary content",
				workspace: t.TempDir(),
				path:      "binary.bin",
				content:   "aGVsbG8=", // base64 of "hello"
				encoding:  "base64",
				wantErr:   false,
				checkFn: func(t *testing.T, wsDir string) {
					absPath := filepath.Join(wsDir, "binary.bin")
					data, err := os.ReadFile(absPath)
					if err != nil {
						t.Fatalf("ReadFile: %v", err)
					}
					if string(data) != "hello" {
						t.Errorf("content = %q, want %q", string(data), "hello")
					}
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				os.Unsetenv("MEWORK_WORKSPACE_PATH")

				if tt.workspace != "" {
					t.Setenv("MEWORK_WORKSPACE_PATH", tt.workspace)
				}

				srv := setupSessionTools(t)
				defer srv.Close()

				args := map[string]interface{}{
					"path":    tt.path,
					"content": tt.content,
				}
				if tt.encoding != "" {
					args["encoding"] = tt.encoding
				}

				result, err := srv.Client().CallTool(t.Context(), mcp.CallToolRequest{
					Params: mcp.CallToolParams{
						Name:      "write_artifact",
						Arguments: args,
					},
				})

				if tt.wantErr {
					if err == nil {
						t.Error("expected error, got nil")
					}
					return
				}

				if err != nil {
					t.Fatalf("CallTool: %v", err)
				}
				if result.IsError {
					t.Fatal("expected success, got isError=true")
				}

				// Verify the response shape.
				m := parseResult(t, result)
				gotPath, ok := m["path"].(string)
				if !ok || gotPath == "" {
					t.Error("expected non-empty path in result")
				}
				size, _ := m["size"].(float64)
				if size <= 0 {
					t.Errorf("expected positive size, got %v", size)
				}

				if tt.checkFn != nil {
					tt.checkFn(t, tt.workspace)
				}
			})
		}
	})
}
