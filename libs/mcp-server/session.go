package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// SessionContext holds the orchestrator's session identity and workspace info,
// populated from environment variables at call time.
type SessionContext struct {
	SessionID       string `json:"session_id"`
	Owner           string `json:"owner"`
	Tenant          string `json:"tenant"`
	WorkspacePath   string `json:"workspace_path"`
	Provider        string `json:"provider"`
	ProviderResource string `json:"provider_resource"`
}

// SessionHandler implements MCP tool handlers for session context tools:
// get_session_context and write_artifact.
type SessionHandler struct{}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler() *SessionHandler {
	return &SessionHandler{}
}

// loadContext reads session identity from environment variables.
func (h *SessionHandler) loadContext() SessionContext {
	return SessionContext{
		SessionID:        os.Getenv("MEWORK_SESSION_ID"),
		Owner:            os.Getenv("MEWORK_SESSION_OWNER"),
		Tenant:           os.Getenv("MEWORK_SESSION_TENANT"),
		WorkspacePath:    os.Getenv("MEWORK_WORKSPACE_PATH"),
		Provider:         os.Getenv("MEWORK_PROVIDER"),
		ProviderResource: os.Getenv("MEWORK_PROVIDER_RESOURCE"),
	}
}

// GetSessionContext handles the get_session_context tool.
// It returns session identity, owner, tenant, workspace path, and provider info
// populated from environment variables at call time.
func (h *SessionHandler) GetSessionContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sc := h.loadContext()
	return map[string]interface{}{
		"session_id":        sc.SessionID,
		"owner":             sc.Owner,
		"tenant":            sc.Tenant,
		"workspace_path":    sc.WorkspacePath,
		"provider":          sc.Provider,
		"provider_resource": sc.ProviderResource,
	}, nil
}

// WriteArtifact handles the write_artifact tool.
//
// Parameters:
//   - path (string, required): relative path within the workspace
//   - content (string, required): content to write
//   - encoding (string, optional): "text" (default) or "base64"
//
// Returns { path: absolute_path, size: bytes_written } or an error
// if MEWORK_WORKSPACE_PATH is not set.
func (h *SessionHandler) WriteArtifact(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	workspacePath := os.Getenv("MEWORK_WORKSPACE_PATH")
	if workspacePath == "" {
		return nil, fmt.Errorf("no workspace path: MEWORK_WORKSPACE_PATH not set")
	}

	relPath, _ := args["path"].(string)
	if relPath == "" {
		return nil, fmt.Errorf("path is required")
	}

	content, _ := args["content"].(string)

	encoding, _ := args["encoding"].(string)
	var data []byte
	switch encoding {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("decode base64 content: %w", err)
		}
		data = decoded
	default:
		data = []byte(content)
	}

	absPath := filepath.Join(workspacePath, relPath)

	// Create parent directories.
	parentDir := filepath.Dir(absPath)
	if err := os.MkdirAll(parentDir, 0700); err != nil {
		return nil, fmt.Errorf("create parent directories: %w", err)
	}

	if err := os.WriteFile(absPath, data, 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return map[string]interface{}{
		"path": absPath,
		"size": len(data),
	}, nil
}
