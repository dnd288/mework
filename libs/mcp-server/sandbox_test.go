package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

// fakeSandboxManager implements SandboxManager for testing.
// It records all operations and permits controlled state transitions.
type fakeSandboxManager struct {
	mu       sync.Mutex
	starts   []startRecord
	stops    []string
	destroys []string
	// Simulated sandbox state keyed by sandbox ID.
	statuses map[string]fakeSandboxState
	nextID   int
}

type startRecord struct {
	agentID string
	prompt  string
	image   string
}

type fakeSandboxState struct {
	status string
	result string
}

func newFakeSandboxManager() *fakeSandboxManager {
	return &fakeSandboxManager{
		statuses: make(map[string]fakeSandboxState),
		nextID:   1,
	}
}

func (f *fakeSandboxManager) Start(_ context.Context, agentID, prompt, image string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, startRecord{agentID: agentID, prompt: prompt, image: image})
	id := fmt.Sprintf("fake-sb-%d", f.nextID)
	f.nextID++
	f.statuses[id] = fakeSandboxState{status: "running"}
	return id, nil
}

func (f *fakeSandboxManager) Stop(_ context.Context, sandboxID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, sandboxID)
	return nil
}

func (f *fakeSandboxManager) Destroy(_ context.Context, sandboxID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroys = append(f.destroys, sandboxID)
	delete(f.statuses, sandboxID)
	return nil
}

func (f *fakeSandboxManager) Status(_ context.Context, sandboxID string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.statuses[sandboxID]
	if !ok {
		return "", "", fmt.Errorf("sandbox %q not found", sandboxID)
	}
	return s.status, s.result, nil
}

func (f *fakeSandboxManager) List(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.statuses))
	for id := range f.statuses {
		ids = append(ids, id)
	}
	return ids, nil
}

func (f *fakeSandboxManager) Wait(ctx context.Context, sandboxID string, timeout time.Duration) (string, string, error) {
	f.mu.Lock()
	s, ok := f.statuses[sandboxID]
	f.mu.Unlock()
	if !ok {
		return "", "", fmt.Errorf("sandbox %q not found", sandboxID)
	}
	// Already terminal — return immediately.
	if s.status != "running" {
		return s.status, s.result, nil
	}

	// Simulate running → done after a short delay.
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-time.After(5 * time.Millisecond):
	}

	f.mu.Lock()
	f.statuses[sandboxID] = fakeSandboxState{status: "done", result: "completed"}
	s = f.statuses[sandboxID]
	f.mu.Unlock()
	return s.status, s.result, nil
}

// setResult directly sets a sandbox's status for test scenarios that need
// to simulate completion without going through Wait.
func (f *fakeSandboxManager) setResult(sandboxID, status, result string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.statuses[sandboxID]; ok {
		f.statuses[sandboxID] = fakeSandboxState{status: status, result: result}
	}
}

// parseResult extracts the JSON text content from a CallToolResult.
func parseResult(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty result content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return m
}

// TestSandboxTools exercises the five sandbox lifecycle MCP tools:
//   - spawn_sandbox
//   - get_sandbox_status
//   - list_child_sandboxes
//   - destroy_sandbox
//   - wait_for_sandbox
//
// RED step: fails because NewSandboxHandler (defined in sandbox.go) is not yet
// implemented. The test declares the expected API surface.
func TestSandboxTools(t *testing.T) {
	fake := newFakeSandboxManager()

	// UNDEFINED SYMBOL — this is the RED failure.
	// NewSandboxHandler will be implemented in sandbox.go.
	handler := NewSandboxHandler(fake)
	if handler == nil {
		t.Fatal("handler is nil")
	}

	reg := NewToolRegistry()

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
		handler.SpawnSandbox,
	)

	reg.Register("get_sandbox_status",
		mcp.NewTool("get_sandbox_status",
			mcp.WithDescription("Returns the current status of a child sandbox"),
			mcp.WithString("sandbox_id", mcp.Required(),
				mcp.Description("ID of the sandbox to query")),
		),
		handler.GetSandboxStatus,
	)

	reg.Register("list_child_sandboxes",
		mcp.NewTool("list_child_sandboxes",
			mcp.WithDescription("Lists all active child sandboxes managed by this server process"),
		),
		handler.ListChildSandboxes,
	)

	reg.Register("destroy_sandbox",
		mcp.NewTool("destroy_sandbox",
			mcp.WithDescription("Stops and permanently removes a child sandbox"),
			mcp.WithString("sandbox_id", mcp.Required(),
				mcp.Description("ID of the sandbox to destroy")),
		),
		handler.DestroySandbox,
	)

	reg.Register("wait_for_sandbox",
		mcp.NewTool("wait_for_sandbox",
			mcp.WithDescription("Waits for a child sandbox to reach a terminal state"),
			mcp.WithString("sandbox_id", mcp.Required(),
				mcp.Description("ID of the sandbox to wait for")),
			mcp.WithNumber("timeout_seconds",
				mcp.Description("Max wait time in seconds (default: 300)")),
		),
		handler.WaitForSandbox,
	)

	tools := reg.ServerTools()
	srv, err := mcptest.NewServer(t, tools...)
	if err != nil {
		t.Fatalf("mcptest.NewServer: %v", err)
	}
	defer srv.Close()

	client := srv.Client()

	// callTool is a convenience wrapper for invoking an MCP tool.
	callTool := func(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
		return client.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      name,
				Arguments: args,
			},
		})
	}
	ctx := t.Context()

	// -----------------------------------------------------------------------
	// 1. spawn_sandbox creates and returns a non-empty ID
	// -----------------------------------------------------------------------
	var spawnedID string
	t.Run("spawn_sandbox creates and returns ID", func(t *testing.T) {
		result, err := callTool(ctx, "spawn_sandbox", map[string]interface{}{
			"agent_id": "child-1",
			"prompt":   "implement the login feature",
			"image":    "ubuntu:22.04",
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}
		m := parseResult(t, result)
		id, ok := m["sandbox_id"].(string)
		if !ok || id == "" {
			t.Fatal("expected non-empty sandbox_id in result")
		}
		spawnedID = id

		// Verify the fake manager recorded the Start call.
		fake.mu.Lock()
		if len(fake.starts) != 1 {
			t.Fatalf("expected 1 Start call, got %d", len(fake.starts))
		}
		if fake.starts[0].agentID != "child-1" {
			t.Errorf("agentID = %q, want %q", fake.starts[0].agentID, "child-1")
		}
		if fake.starts[0].prompt != "implement the login feature" {
			t.Errorf("prompt = %q, want %q", fake.starts[0].prompt, "implement the login feature")
		}
		if fake.starts[0].image != "ubuntu:22.04" {
			t.Errorf("image = %q, want %q", fake.starts[0].image, "ubuntu:22.04")
		}
		fake.mu.Unlock()
	})

	// -----------------------------------------------------------------------
	// 2. spawn_sandbox with missing prompt returns error
	// -----------------------------------------------------------------------
	t.Run("spawn_sandbox with missing prompt returns error", func(t *testing.T) {
		_, err := callTool(ctx, "spawn_sandbox", map[string]interface{}{
			"agent_id": "child-2",
			"prompt":   "",
		})
		if err == nil {
			t.Error("expected error for empty prompt, got nil")
		}
	})

	// -----------------------------------------------------------------------
	// 3. get_sandbox_status returns running after spawn
	// -----------------------------------------------------------------------
	t.Run("get_sandbox_status returns running after spawn", func(t *testing.T) {
		if spawnedID == "" {
			t.Skip("no spawned sandbox — previous test may have failed")
		}
		result, err := callTool(ctx, "get_sandbox_status", map[string]interface{}{
			"sandbox_id": spawnedID,
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}
		m := parseResult(t, result)
		status, ok := m["status"].(string)
		if !ok {
			t.Fatal("expected status field in result")
		}
		if status != "running" {
			t.Errorf("status = %q, want %q", status, "running")
		}
	})

	// -----------------------------------------------------------------------
	// 4. get_sandbox_status returns done after completion
	// -----------------------------------------------------------------------
	t.Run("get_sandbox_status returns done after completion", func(t *testing.T) {
		// Simulate the sandbox completing via the fake manager.
		fake.setResult(spawnedID, "done", "login feature implemented")

		result, err := callTool(ctx, "get_sandbox_status", map[string]interface{}{
			"sandbox_id": spawnedID,
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}
		m := parseResult(t, result)
		status, ok := m["status"].(string)
		if !ok {
			t.Fatal("expected status field in result")
		}
		if status != "done" {
			t.Errorf("status = %q, want %q", status, "done")
		}
		res, _ := m["result"].(string)
		if res != "login feature implemented" {
			t.Errorf("result = %q, want %q", res, "login feature implemented")
		}
	})

	// -----------------------------------------------------------------------
	// 5. get_sandbox_status for unknown ID returns error
	// -----------------------------------------------------------------------
	t.Run("get_sandbox_status for unknown ID returns not_found", func(t *testing.T) {
		_, err := callTool(ctx, "get_sandbox_status", map[string]interface{}{
			"sandbox_id": "nonexistent-sandbox",
		})
		if err == nil {
			t.Error("expected error for unknown sandbox_id, got nil")
		}
	})

	// -----------------------------------------------------------------------
	// 6. list_child_sandboxes returns all active
	// -----------------------------------------------------------------------
	t.Run("list_child_sandboxes returns all active", func(t *testing.T) {
		// Spawn two more children so we have three total.
		for i := 0; i < 2; i++ {
			_, err := callTool(ctx, "spawn_sandbox", map[string]interface{}{
				"agent_id": fmt.Sprintf("child-%d", i+2),
				"prompt":   "some task",
			})
			if err != nil {
				t.Fatalf("spawn child %d: %v", i+2, err)
			}
		}

		result, err := callTool(ctx, "list_child_sandboxes", map[string]interface{}{})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}
		m := parseResult(t, result)
		children, ok := m["children"].([]interface{})
		if !ok {
			t.Fatal("expected children array in result")
		}
		if len(children) != 3 {
			t.Errorf("expected 3 children, got %d", len(children))
		}
		// Each child should have sandbox_id, agent_id, status, created_at.
		for i, c := range children {
			child, ok := c.(map[string]interface{})
			if !ok {
				t.Errorf("child[%d] is not a map", i)
				continue
			}
			if _, ok := child["sandbox_id"]; !ok {
				t.Errorf("child[%d] missing sandbox_id", i)
			}
			if _, ok := child["agent_id"]; !ok {
				t.Errorf("child[%d] missing agent_id", i)
			}
			if _, ok := child["status"]; !ok {
				t.Errorf("child[%d] missing status", i)
			}
		}
	})

	// -----------------------------------------------------------------------
	// 7. destroy_sandbox stops and removes
	// -----------------------------------------------------------------------
	t.Run("destroy_sandbox stops and removes", func(t *testing.T) {
		if spawnedID == "" {
			t.Skip("no spawned sandbox")
		}
		result, err := callTool(ctx, "destroy_sandbox", map[string]interface{}{
			"sandbox_id": spawnedID,
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}

		// Verify the fake manager received Stop and Destroy calls.
		fake.mu.Lock()
		if len(fake.stops) != 1 {
			t.Errorf("expected 1 Stop call, got %d", len(fake.stops))
		}
		if len(fake.destroys) != 1 {
			t.Errorf("expected 1 Destroy call, got %d", len(fake.destroys))
		}
		// The sandbox should be removed from the active set.
		if _, exists := fake.statuses[spawnedID]; exists {
			t.Error("expected sandbox to be removed from active statuses")
		}
		fake.mu.Unlock()

		// Subsequent get_sandbox_status should return not_found.
		_, err = callTool(ctx, "get_sandbox_status", map[string]interface{}{
			"sandbox_id": spawnedID,
		})
		if err == nil {
			t.Error("expected error for destroyed sandbox, got nil")
		}
	})

	// -----------------------------------------------------------------------
	// 8. destroy_sandbox for unknown ID returns error
	// -----------------------------------------------------------------------
	t.Run("destroy_sandbox for unknown ID returns error", func(t *testing.T) {
		_, err := callTool(ctx, "destroy_sandbox", map[string]interface{}{
			"sandbox_id": "nonexistent-sandbox",
		})
		if err == nil {
			t.Error("expected error for unknown sandbox_id, got nil")
		}
	})

	// -----------------------------------------------------------------------
	// 9. wait_for_sandbox blocks until done
	// -----------------------------------------------------------------------
	t.Run("wait_for_sandbox blocks until done", func(t *testing.T) {
		// Spawn a new sandbox, then wait for it.
		spawnResult, err := callTool(ctx, "spawn_sandbox", map[string]interface{}{
			"agent_id": "child-wait",
			"prompt":   "short-lived task",
		})
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
		m := parseResult(t, spawnResult)
		waitID, _ := m["sandbox_id"].(string)
		if waitID == "" {
			t.Fatal("expected non-empty sandbox_id")
		}

		result, err := callTool(ctx, "wait_for_sandbox", map[string]interface{}{
			"sandbox_id":     waitID,
			"timeout_seconds": 30.0,
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if result.IsError {
			t.Fatal("expected success, got isError=true")
		}
		m = parseResult(t, result)
		status, ok := m["status"].(string)
		if !ok {
			t.Fatal("expected status field in result")
		}
		if status != "done" {
			t.Errorf("status = %q, want %q", status, "done")
		}
		res, _ := m["result"].(string)
		if res == "" {
			t.Error("expected non-empty result")
		}
	})

	// -----------------------------------------------------------------------
	// 10. wait_for_sandbox times out
	// -----------------------------------------------------------------------
	t.Run("wait_for_sandbox times out", func(t *testing.T) {
		// Spawn a sandbox — the fake will complete it after 5ms normally.
		// For the timeout case, the handler should respect a very short timeout
		// and return a timeout error before the fake completes.
		spawnResult, err := callTool(ctx, "spawn_sandbox", map[string]interface{}{
			"agent_id": "child-timeout",
			"prompt":   "long task",
		})
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
		m := parseResult(t, spawnResult)
		timeoutID, _ := m["sandbox_id"].(string)
		if timeoutID == "" {
			t.Fatal("expected non-empty sandbox_id")
		}

		// Use a 1ms timeout — the handler should return a timeout error
		// before the fake's 5ms simulated delay completes.
		result, err := callTool(ctx, "wait_for_sandbox", map[string]interface{}{
			"sandbox_id":     timeoutID,
			"timeout_seconds": 0.001, // 1ms
		})
		if err == nil {
			if result.IsError {
				return // server-side error is acceptable
			}
			t.Error("expected timeout error, got nil")
		}
	})
}
