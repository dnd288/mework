package mello

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a typed REST client for the Mello API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	// clientVersion tags requests via X-Client-Version for server-side metrics.
	clientVersion string
}

// NewClient builds a client. baseURL defaults to the v1 API; timeout 0 → 30s.
func NewClient(baseURL, token string, timeout time.Duration, clientVersion string) *Client {
	if baseURL == "" {
		baseURL = "https://mello.mezon.vn/api/v1"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		token:         token,
		httpClient:    &http.Client{Timeout: timeout},
		clientVersion: clientVersion,
	}
}

// resolveURL builds the full URL. When useV1 is false and the base ends in
// /v1, the suffix is stripped (checklists/attachments live under /api).
func (c *Client) resolveURL(path string, useV1 bool) string {
	base := c.baseURL
	if !useV1 && strings.HasSuffix(base, "/v1") {
		base = base[:len(base)-3]
	}
	return base + path
}

// do performs a request and decodes a JSON response into out (may be nil).
// Non-2xx responses are decoded into *APIError.
func (c *Client) do(method, path string, useV1 bool, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.resolveURL(path, useV1), reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.clientVersion != "" {
		req.Header.Set("X-Client-Version", c.clientVersion)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, data)
	}
	if resp.StatusCode == http.StatusNoContent || len(data) == 0 || out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// parseAPIError decodes an error body into *APIError, tolerating empty/invalid JSON.
func parseAPIError(status int, data []byte) error {
	apiErr := &APIError{StatusCode: status, ErrorCode: "unknown_error", Message: "an unexpected error occurred"}
	if len(data) > 0 {
		_ = json.Unmarshal(data, apiErr) // best-effort; keep defaults on failure
	}
	if apiErr.Message == "" {
		apiErr.Message = apiErr.ErrorCode
	}
	return apiErr
}
