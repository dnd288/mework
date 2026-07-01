package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ChildSandbox holds the metadata for a tracked child sandbox.
type ChildSandbox struct {
	AgentID   string    `json:"agent_id"`
	CreatedAt time.Time `json:"created_at"`
}

// SandboxManager defines the interface for sandbox lifecycle operations.
// The production implementation wraps libs/sandbox/runtime.Manager.
// The test file's fakeSandboxManager implements this same interface.
type SandboxManager interface {
	Start(ctx context.Context, agentID, prompt, image string) (sandboxID string, err error)
	Stop(ctx context.Context, sandboxID string) error
	Destroy(ctx context.Context, sandboxID string) error
	Status(ctx context.Context, sandboxID string) (status, result string, err error)
	List(ctx context.Context) ([]string, error)
	Wait(ctx context.Context, sandboxID string, timeout time.Duration) (status, result string, err error)
}

// SandboxHandler implements MCP tool handlers for sandbox lifecycle tools.
type SandboxHandler struct {
	mu      sync.Mutex
	manager SandboxManager
	infos   map[string]ChildSandbox
}

// NewSandboxHandler creates a new SandboxHandler backed by the given manager.
func NewSandboxHandler(manager SandboxManager) *SandboxHandler {
	return &SandboxHandler{
		manager: manager,
		infos:   make(map[string]ChildSandbox),
	}
}

// SpawnSandbox handles the spawn_sandbox tool.
func (h *SandboxHandler) SpawnSandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	agentID, _ := args["agent_id"].(string)
	prompt, _ := args["prompt"].(string)
	image, _ := args["image"].(string)

	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if image == "" {
		image = "ubuntu:22.04"
	}

	sandboxID, err := h.manager.Start(ctx, agentID, prompt, image)
	if err != nil {
		return nil, fmt.Errorf("start sandbox: %w", err)
	}

	h.mu.Lock()
	h.infos[sandboxID] = ChildSandbox{
		AgentID:   agentID,
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()

	return map[string]string{"sandbox_id": sandboxID}, nil
}

// GetSandboxStatus handles the get_sandbox_status tool.
func (h *SandboxHandler) GetSandboxStatus(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	status, result, err := h.manager.Status(ctx, sandboxID)
	if err != nil {
		return nil, err
	}

	resp := map[string]interface{}{
		"status": status,
		"result": result,
	}

	h.mu.Lock()
	if info, ok := h.infos[sandboxID]; ok {
		resp["created_at"] = info.CreatedAt.Format(time.RFC3339)
	}
	h.mu.Unlock()

	return resp, nil
}

// ListChildSandboxes handles the list_child_sandboxes tool.
func (h *SandboxHandler) ListChildSandboxes(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	ids, err := h.manager.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}

	children := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		status, result, err := h.manager.Status(ctx, id)
		if err != nil {
			continue
		}

		child := map[string]interface{}{
			"sandbox_id": id,
			"status":     status,
			"result":     result,
		}

		h.mu.Lock()
		if info, ok := h.infos[id]; ok {
			child["agent_id"] = info.AgentID
			child["created_at"] = info.CreatedAt.Format(time.RFC3339)
		}
		h.mu.Unlock()

		children = append(children, child)
	}

	return map[string]interface{}{"children": children}, nil
}

// DestroySandbox handles the destroy_sandbox tool.
func (h *SandboxHandler) DestroySandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	// Verify the sandbox exists by querying status.
	_, _, err := h.manager.Status(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox not found: %s", sandboxID)
	}

	if err := h.manager.Stop(ctx, sandboxID); err != nil {
		return nil, fmt.Errorf("stop sandbox: %w", err)
	}

	if err := h.manager.Destroy(ctx, sandboxID); err != nil {
		return nil, fmt.Errorf("destroy sandbox: %w", err)
	}

	h.mu.Lock()
	delete(h.infos, sandboxID)
	h.mu.Unlock()

	return map[string]string{"status": "destroyed"}, nil
}

// WaitForSandbox handles the wait_for_sandbox tool.
func (h *SandboxHandler) WaitForSandbox(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sandboxID, _ := args["sandbox_id"].(string)
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	var timeout time.Duration
	if timeoutSeconds, ok := args["timeout_seconds"].(float64); ok && timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds * float64(time.Second))
	} else {
		timeout = 300 * time.Second
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	status, result, err := h.manager.Wait(waitCtx, sandboxID, timeout)
	if err != nil {
		if waitCtx.Err() != nil {
			return nil, fmt.Errorf("timeout waiting for sandbox %s", sandboxID)
		}
		return nil, err
	}

	return map[string]interface{}{
		"status": status,
		"result": result,
	}, nil
}
