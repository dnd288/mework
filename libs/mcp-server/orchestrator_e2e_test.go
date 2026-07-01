// Package main implements the mework MCP server binary.
//
// End-to-end integration tests for the orchestrator MCP tools. These tests
// exercise the full lifecycle: create an orchestrator with all tools registered,
// spawn a child sandbox, wait for completion, notify human, query session context,
// and verify child sandbox listing — all through the MCP protocol layer.
//
// Run with: TEST_INTEGRATION=true go test ./libs/mcp-server/... -v
package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"
)

// registerAllTools creates a ToolRegistry with every MCP tool registered and
// returns the tools suitable for mcptest.NewServer.
func registerAllTools(t *testing.T, sandboxMgr SandboxManager, bus BusBroker) []server.ServerTool {
	t.Helper()

	sh := NewSandboxHandler(sandboxMgr)
	nh := NewNotifyHandler(bus)
	sessh := NewSessionHandler()

	reg := NewToolRegistry()

	// Sandbox lifecycle tools
	reg.Register("spawn_sandbox",
		mcp.NewTool("spawn_sandbox",
			mcp.WithDescription("Spawns a child sandbox agent for delegated work"),
			mcp.WithString("agent_id", mcp.Required(),
				mcp.Description("Unique identifier for the child agent")),
			mcp.WithString("prompt", mcp.Required(),
				mcp.Description("Instructions for the child agent; fed over stdin")),
			mcp.WithString("image",
				mcp.Description("Container image (default: ubuntu:22.04)")),
			mcp.WithNumber("timeout_minutes",
				mcp.Description("Max run time in minutes (default: 30)")),
			mcp.WithString("workspace_path",
				mcp.Description("Workspace directory for the child")),
		),
		sh.SpawnSandbox,
	)

	reg.Register("get_sandbox_status",
		mcp.NewTool("get_sandbox_status",
			mcp.WithDescription("Returns the current status of a child sandbox"),
			mcp.WithString("sandbox_id", mcp.Required(),
				mcp.Description("ID of the sandbox to query")),
		),
		sh.GetSandboxStatus,
	)

	reg.Register("list_child_sandboxes",
		mcp.NewTool("list_child_sandboxes",
			mcp.WithDescription("Lists all active child sandboxes managed by this server process"),
		),
		sh.ListChildSandboxes,
	)

	reg.Register("destroy_sandbox",
		mcp.NewTool("destroy_sandbox",
			mcp.WithDescription("Stops and permanently removes a child sandbox"),
			mcp.WithString("sandbox_id", mcp.Required(),
				mcp.Description("ID of the sandbox to destroy")),
		),
		sh.DestroySandbox,
	)

	reg.Register("wait_for_sandbox",
		mcp.NewTool("wait_for_sandbox",
			mcp.WithDescription("Waits for a child sandbox to reach a terminal state"),
			mcp.WithString("sandbox_id", mcp.Required(),
				mcp.Description("ID of the sandbox to wait for")),
			mcp.WithNumber("timeout_seconds",
				mcp.Description("Max wait time in seconds (default: 300)")),
		),
		sh.WaitForSandbox,
	)

	// Notification tools
	reg.Register("notify_human",
		mcp.NewTool("notify_human",
			mcp.WithDescription("Sends a notification to the human user"),
			mcp.WithString("message", mcp.Required(),
				mcp.Description("The message content to send")),
			mcp.WithString("format",
				mcp.Description("Message format: text or markdown")),
			mcp.WithArray("attachments",
				mcp.WithStringItems(mcp.Description("Optional file attachments"))),
		),
		nh.NotifyHuman,
	)

	reg.Register("ask_human",
		mcp.NewTool("ask_human",
			mcp.WithDescription("Asks the human a question and waits for a response"),
			mcp.WithString("question", mcp.Required(),
				mcp.Description("The question to ask the human")),
			mcp.WithArray("options",
				mcp.WithStringItems(mcp.Description("Valid response options"))),
			mcp.WithNumber("timeout_minutes",
				mcp.Description("Max time to wait for a response in minutes")),
		),
		nh.AskHuman,
	)

	// Session context tools
	reg.Register("get_session_context",
		mcp.NewTool("get_session_context",
			mcp.WithDescription("Returns session context identity and workspace info"),
		),
		sessh.GetSessionContext,
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
		sessh.WriteArtifact,
	)

	return reg.ServerTools()
}

// TestOrchestratorE2E exercises the full orchestrator lifecycle through the
// MCP protocol layer. It simulates an orchestrator agent: spawning a child
// sandbox, waiting for it, notifying the human, querying session context, and
// listing children — all via the registered MCP tools.
//
// Gated behind TEST_INTEGRATION=true because it exercises the full tool stack
// through the MCP transport layer.
func TestOrchestratorE2E(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "true" {
		t.Skip("set TEST_INTEGRATION=true to run orchestrator e2e integration tests")
	}

	// Sub-tests share an mcptest.Server to simulate a continuous orchestrator session.
	bus := newFakeBus()
	fake := newFakeSandboxManager()

	t.Setenv("MEWORK_SESSION_ID", "test-e2e")

	tools := registerAllTools(t, fake, bus)

	srv, err := mcptest.NewServer(t, tools...)
	if err != nil {
		t.Fatalf("mcptest.NewServer: %v", err)
	}
	defer srv.Close()

	client := srv.Client()

	// callTool is a convenience wrapper for invoking an MCP tool through the client.
	callTool := func(t *testing.T, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
		t.Helper()
		ctx := t.Context()
		return client.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      name,
				Arguments: args,
			},
		})
	}

	ctx := t.Context()

	// -----------------------------------------------------------------------
	// Table: Orchestrator lifecycle steps
	// -----------------------------------------------------------------------
	t.Run("full orchestrator lifecycle", func(t *testing.T) {
		type lifecycleStep struct {
			name         string
			tool         string
			args         map[string]interface{}
			wantErr      bool
			check        func(t *testing.T, result *mcp.CallToolResult) string // returns sandbox_id for pipeline
		}

		var sandboxID string

		steps := []lifecycleStep{
			{
				name: "spawn child sandbox",
				tool: "spawn_sandbox",
				args: map[string]interface{}{
					"agent_id": "child-e2e",
					"prompt":   "echo hello from child",
				},
				wantErr: false,
				check: func(t *testing.T, result *mcp.CallToolResult) string {
					m := parseResult(t, result)
					id, ok := m["sandbox_id"].(string)
					if !ok || id == "" {
						t.Fatal("expected non-empty sandbox_id")
					}
					return id
				},
			},
			{
				name: "wait for child completion",
				tool: "wait_for_sandbox",
				args: func() map[string]interface{} {
					return map[string]interface{}{
						"sandbox_id":     sandboxID,
						"timeout_seconds": 30.0,
					}
				}(),
				wantErr: false,
				check: func(t *testing.T, result *mcp.CallToolResult) string {
					m := parseResult(t, result)
					status, ok := m["status"].(string)
					if !ok {
						t.Fatal("expected status field")
					}
					if status != "done" {
						t.Errorf("status = %q, want %q", status, "done")
					}
					res, _ := m["result"].(string)
					if res == "" {
						t.Error("expected non-empty result")
					}
					return ""
				},
			},
			{
				name: "notify human of completion",
				tool: "notify_human",
				args: map[string]interface{}{
					"message": "child done",
					"format":  "text",
				},
				wantErr: false,
				check: func(t *testing.T, result *mcp.CallToolResult) string {
					// Verify bus received the notification
					record, ok := bus.lastPublish()
					if !ok {
						t.Fatal("expected bus publish for notify_human")
					}
					if record.topic != "session.test-e2e.output" {
						t.Errorf("topic = %q, want %q", record.topic, "session.test-e2e.output")
					}
					var payload map[string]interface{}
					if err := json.Unmarshal(record.payload, &payload); err != nil {
						t.Fatalf("unmarshal payload: %v", err)
					}
					msg, _ := payload["message"].(string)
					if msg != "child done" {
						t.Errorf("message = %q, want %q", msg, "child done")
					}
					return ""
				},
			},
			{
				name: "get session context returns session_id",
				tool: "get_session_context",
				args: map[string]interface{}{},
				wantErr: false,
				check: func(t *testing.T, result *mcp.CallToolResult) string {
					m := parseResult(t, result)
					sid, ok := m["session_id"].(string)
					if !ok || sid == "" {
						t.Error("expected non-empty session_id")
					}
					if sid != "test-e2e" {
						t.Errorf("session_id = %q, want %q", sid, "test-e2e")
					}
					return ""
				},
			},
			{
				name: "child sandbox appears in listing",
				tool: "list_child_sandboxes",
				args: map[string]interface{}{},
				wantErr: false,
				check: func(t *testing.T, result *mcp.CallToolResult) string {
					m := parseResult(t, result)
					children, ok := m["children"].([]interface{})
					if !ok {
						t.Fatal("expected children array in result")
					}
					if len(children) == 0 {
						t.Fatal("expected at least one child sandbox in listing")
					}
					// Verify the spawned child appears
					found := false
					for _, c := range children {
						child, ok := c.(map[string]interface{})
						if !ok {
							continue
						}
						id, _ := child["sandbox_id"].(string)
						if id == sandboxID {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected sandbox %q to appear in list_child_sandboxes", sandboxID)
					}
					return ""
				},
			},
		}

		// Run the lifecycle steps in order. The first step's check captures
		// sandboxID for use by subsequent steps that reference it.
		for _, step := range steps {
			t.Run(step.name, func(t *testing.T) {
				// If the step references sandboxID from a previous step, it's
				// embedded directly in the args closure above via the func() trick.
				// However, sandboxID is captured by reference; since steps[0].check
				// sets it, the later steps that use sandboxID in their args closure
				// need the variable to be already populated. We handle this by
				// filling in sandboxID after step 0 runs.
				if step.tool == "wait_for_sandbox" && sandboxID != "" {
					step.args = map[string]interface{}{
						"sandbox_id":     sandboxID,
						"timeout_seconds": 30.0,
					}
				}

				result, err := callTool(t, step.tool, step.args)
				if step.wantErr {
					if err == nil {
						t.Errorf("expected error for tool %q, got nil", step.tool)
					}
					return
				}
				if err != nil {
					t.Fatalf("CallTool %q: %v", step.tool, err)
				}
				if result.IsError {
					t.Fatalf("CallTool %q returned isError=true", step.tool)
				}

				if step.check != nil {
					if gotID := step.check(t, result); gotID != "" {
						sandboxID = gotID
					}
				}
			})
		}
	})

	// -----------------------------------------------------------------------
	// Table: ask_human round-trip scenarios
	// -----------------------------------------------------------------------
	t.Run("ask_human round-trip", func(t *testing.T) {
		tests := []struct {
			name       string
			question   string
			options    []string
			response   string
			wantErr    bool
			timeoutMin float64
		}{
			{
				name:       "asks question and receives valid response",
				question:   "proceed?",
				options:    []string{"yes", "no"},
				response:   "yes",
				wantErr:    false,
				timeoutMin: 1.0,
			},
			{
				name:       "rejects invalid option and waits for valid",
				question:   "color?",
				options:    []string{"blue", "green"}, // "red" is NOT valid
				response:   "blue", // valid; we send "red" first (invalid), then "blue" (valid)
				wantErr:    false,
				timeoutMin: 1.0,
			},
			{
				name:       "times out with no response",
				question:   "quick?",
				options:    []string{"y", "n"},
				response:   "",
				wantErr:    true,
				timeoutMin: 0.01, // ~600ms, but the handler should time out first
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Subscribe to the input topic before calling ask_human.
				respCh, err := bus.Subscribe(ctx, "session.test-e2e.input")
				if err != nil {
					t.Fatalf("Subscribe: %v", err)
				}

				type callResult struct {
					resp *mcp.CallToolResult
					err  error
				}
				resultCh := make(chan callResult, 1)

				// Call ask_human in a goroutine (it blocks).
				go func() {
					r, e := client.CallTool(ctx, mcp.CallToolRequest{
						Params: mcp.CallToolParams{
							Name: "ask_human",
							Arguments: map[string]interface{}{
								"question":       tt.question,
								"options":        tt.options,
								"timeout_minutes": tt.timeoutMin,
							},
						},
					})
					resultCh <- callResult{resp: r, err: e}
				}()

				if tt.wantErr {
					// For timeout test: wait for result, expect error.
					select {
					case r := <-resultCh:
						if r.err == nil && !r.resp.IsError {
							t.Error("expected error for timeout, got success")
						}
					case <-time.After(5 * time.Second):
						t.Fatal("timeout waiting for ask_human result")
					}
					return
				}

				// For the "rejects invalid option" case, send an invalid response first.
				if tt.name == "rejects invalid option and waits for valid" {
					select {
					case <-respCh:
						invalidPayload, _ := json.Marshal(map[string]string{"response": "red"})
						if err := bus.Publish(ctx, "session.test-e2e.input", invalidPayload); err != nil {
							t.Errorf("publish invalid response: %v", err)
						}
						time.Sleep(100 * time.Millisecond)
					case <-time.After(5 * time.Second):
						t.Fatal("timeout waiting for question publication")
					}
				}

				// Wait for question, then send response.
				select {
				case <-respCh:
					respPayload, _ := json.Marshal(map[string]string{"response": tt.response})
					if err := bus.Publish(ctx, "session.test-e2e.input", respPayload); err != nil {
						t.Errorf("publish response: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timeout waiting for question publication on bus")
				}

				// Wait for ask_human to return.
				select {
				case r := <-resultCh:
					if r.err != nil {
						t.Fatalf("CallTool: %v", r.err)
					}
					if r.resp.IsError {
						t.Fatal("expected success, got isError=true")
					}
					if len(r.resp.Content) == 0 {
						t.Fatal("empty result content")
					}
					tc, ok := r.resp.Content[0].(mcp.TextContent)
					if !ok {
						t.Fatalf("expected TextContent, got %T", r.resp.Content[0])
					}
					var respData map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Text), &respData); err != nil {
						t.Fatalf("unmarshal result: %v", err)
					}
					if got, want := respData["response"], tt.response; got != want {
						t.Errorf("response = %v, want %q", got, want)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("timeout waiting for ask_human result")
				}
			})
		}
	})
}
