// Package cloudflare implements the ports.SandboxDriver interface for
// Cloudflare Workers AI remote sandbox execution. The driver dispatches agent
// runs to Cloudflare's edge network for isolated, short-lived execution.
//
// This implementation calls the Cloudflare Workers AI REST API.
// It requires CLOUDFLARE_API_TOKEN and optionally CLOUDFLARE_ACCOUNT_ID
// environment variables to be set.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// Driver implements ports.SandboxDriver for Cloudflare Workers AI.
type Driver struct {
	httpClient *http.Client
	apiToken   string
	accountID  string
}

// New creates a new Cloudflare Driver.
func New() *Driver {
	return &Driver{
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		apiToken:   os.Getenv("CLOUDFLARE_API_TOKEN"),
		accountID:  os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
	}
}

// Caps returns the capabilities of this driver.
func (d *Driver) Caps() core.SandboxCaps {
	return core.SandboxCaps{
		IsIsolated:  true,
		IsRemote:    true,
		SupportsGPU: false,
		SupportsNet: false,
		MaxMemoryMB: 1024,
		MaxDiskMB:   1024,
		DriverName:  "cloudflare",
	}
}

// cloudflareSandbox represents a remote sandbox running on Cloudflare.
type cloudflareSandbox struct {
	id          string
	runID       string
	accountID   string
	apiToken    string
	httpClient  *http.Client
}

func (s *cloudflareSandbox) ID() string { return s.id }

// Exec invokes the agent on Cloudflare Workers AI via the text-generation API.
// The prompt is sent as the input and the output is captured.
func (s *cloudflareSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	prompt, err := io.ReadAll(stdin)
	if err != nil {
		return -1, fmt.Errorf("read stdin: %w", err)
	}

	// Build the request to Cloudflare Workers AI text generation.
	model := "meta/llama-2-7b-chat-int8"
	if len(command) > 1 && command[0] == "model:" {
		model = command[1]
	}

	body := map[string]any{
		"prompt": string(prompt),
		"stream": false,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return -1, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s", s.accountID, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return -1, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return -1, fmt.Errorf("cloudflare api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("cloudflare api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Write the response output.
	if _, err := stdout.Write(respBody); err != nil {
		return -1, fmt.Errorf("write stdout: %w", err)
	}
	return 0, nil
}

// Mount is a no-op for the remote driver — filesystem operations are not supported.
func (s *cloudflareSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	return fmt.Errorf("cloudflare driver does not support filesystem mounts")
}

// Signals is a no-op for the remote driver.
func (s *cloudflareSandbox) Signals(ctx context.Context, sig string) error {
	return fmt.Errorf("cloudflare driver does not support signals")
}

// Start validates that the Cloudflare credentials are available and returns
// a sandbox placeholder. The actual remote execution happens in Exec.
func (d *Driver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	if d.apiToken == "" {
		return nil, fmt.Errorf("CLOUDFLARE_API_TOKEN is not set")
	}
	if d.accountID == "" {
		return nil, fmt.Errorf("CLOUDFLARE_ACCOUNT_ID is not set")
	}
	return &cloudflareSandbox{
		id:         spec.SandboxID,
		runID:      spec.AgentID,
		accountID:  d.accountID,
		apiToken:   d.apiToken,
		httpClient: d.httpClient,
	}, nil
}

// Stop is a no-op for the remote driver.
func (d *Driver) Stop(ctx context.Context, sandboxID string) error { return nil }

// Destroy is a no-op for the remote driver — no local resources to clean.
func (d *Driver) Destroy(ctx context.Context, sandboxID string) error { return nil }
