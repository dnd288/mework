// Package mcp wraps the hosted Mello MCP server (HTTP/SSE streamable transport)
// for the daemon's write-back operations: posting comments and updating
// checklists after an agent run.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ErrNoURL indicates the required mcp_url config value is unset.
var ErrNoURL = errors.New("mcp url is required but not configured (set mcp_url via `mello config set mcp_url <endpoint>`)")

// Client wraps a streamable-HTTP MCP client bound to the hosted Mello MCP.
type Client struct {
	c *mcpclient.Client
}

// New dials the hosted Mello MCP at url, authenticating with the bearer token.
// The url is required; an empty value returns ErrNoURL. Initialize is performed
// here so callers get a ready-to-use client.
func New(ctx context.Context, url, token string, timeout time.Duration) (*Client, error) {
	if url == "" {
		return nil, ErrNoURL
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	headers := map[string]string{"Authorization": "Bearer " + token}
	cli, err := mcpclient.NewStreamableHttpClient(
		url,
		transport.WithHTTPHeaders(headers),
		transport.WithHTTPTimeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create mcp client: %w", err)
	}
	if err := cli.Start(ctx); err != nil {
		return nil, fmt.Errorf("start mcp transport: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "mello-cli", Version: "dev"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("initialize mcp session: %w", err)
	}
	return &Client{c: cli}, nil
}

// Close shuts down the underlying transport.
func (m *Client) Close() error {
	if m.c == nil {
		return nil
	}
	return m.c.Close()
}

// call invokes a tool and returns its first text content, erroring on tool failure.
func (m *Client) call(ctx context.Context, name string, args map[string]any) (string, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	res, err := m.c.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("call %s: %w", name, err)
	}
	if res.IsError {
		return "", fmt.Errorf("tool %s returned an error: %s", name, firstText(res))
	}
	return firstText(res), nil
}

// firstText extracts the first text block from a tool result, if any.
func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
