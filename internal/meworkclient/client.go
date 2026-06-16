package meworkclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) do(method, path string, token string, body, out any) (int, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return 0, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(data))
	}

	if resp.StatusCode == http.StatusNoContent || out == nil {
		return resp.StatusCode, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}

	if err := json.Unmarshal(data, out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}

	return resp.StatusCode, nil
}
