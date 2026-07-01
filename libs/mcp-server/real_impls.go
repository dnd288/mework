package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// hubClient is an HTTP client for the mework hub API. Used by RealSandboxManager
// and RealBusBroker to call hub endpoints for session lifecycle and bus operations.
type hubClient struct {
	baseURL string
	token   string // PAT or runtime token
	http    *http.Client
}

func newHubClient() *hubClient {
	baseURL := os.Getenv("MEWORK_HUB_URL")
	if baseURL == "" {
		baseURL = os.Getenv("MEWORK_DAEMON_ADDR")
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	token := os.Getenv("MEWORK_PAT")
	if token == "" {
		token = os.Getenv("MEWORK_RT_TOKEN")
	}
	return &hubClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *hubClient) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.http.Do(req)
}

func (c *hubClient) doJSON(method, path string, payload, out interface{}) error {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	resp, err := c.do(method, path, reqBody)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// RealSandboxManager
// ---------------------------------------------------------------------------

// RealSandboxManager implements SandboxManager by calling the hub's session API.
// Each spawned sandbox is backed by a hub session; attributes are stored in the
// session's metadata. The MCP server reads MEWORK_HUB_URL and MEWORK_PAT (or
// MEWORK_RT_TOKEN) from the environment to authenticate.
type RealSandboxManager struct {
	client *hubClient
}

func NewRealSandboxManager() *RealSandboxManager {
	return &RealSandboxManager{client: newHubClient()}
}

type sessionInfo struct {
	ID     string `json:"ID"`
	Tenant string `json:"Tenant"`
	Owner  string `json:"Owner"`
	Runner string `json:"Runner"`
	Agent  struct {
		ID   string `json:"ID"`
		Kind string `json:"Kind"`
		Name string `json:"Name"`
	} `json:"Agent"`
	Status    string `json:"Status"`
	CreatedAt string `json:"Created"`
}

type createSessionReq struct {
	AgentName string `json:"agent_name"`
	Runner    string `json:"runner"`
	Workspace string `json:"workspace,omitempty"`
}

// Start creates a new session on the hub as a backing sandbox.
func (m *RealSandboxManager) Start(ctx context.Context, agentID, prompt, image string) (string, error) {
	runner := os.Getenv("MEWORK_RUNNER_ID")
	if runner == "" {
		return "", fmt.Errorf("MEWORK_RUNNER_ID not set — sandbox operations need a registered runner")
	}

	var info sessionInfo
	req := createSessionReq{
		AgentName: agentID,
		Runner:    runner,
	}
	if err := m.client.doJSON("POST", "/api/v1/sessions", req, &info); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	if info.ID == "" {
		return "", fmt.Errorf("create session returned empty id")
	}
	log.Printf("RealSandboxManager: created session %s for agent %s", info.ID, agentID)
	return info.ID, nil
}

func (m *RealSandboxManager) Stop(ctx context.Context, sandboxID string) error {
	return m.client.doJSON("DELETE", fmt.Sprintf("/api/v1/sessions/%s", sandboxID), nil, nil)
}

func (m *RealSandboxManager) Destroy(ctx context.Context, sandboxID string) error {
	return m.client.doJSON("DELETE", fmt.Sprintf("/api/v1/sessions/%s", sandboxID), nil, nil)
}

func (m *RealSandboxManager) Status(ctx context.Context, sandboxID string) (string, string, error) {
	var info sessionInfo
	if err := m.client.doJSON("GET", fmt.Sprintf("/api/v1/sessions/%s", sandboxID), nil, &info); err != nil {
		return "", "", fmt.Errorf("get session: %w", err)
	}
	return info.Status, fmt.Sprintf("agent=%s runner=%s", info.Agent, info.Runner), nil
}

func (m *RealSandboxManager) List(ctx context.Context) ([]string, error) {
	var sessions []sessionInfo
	if err := m.client.doJSON("GET", "/api/v1/sessions?status=active", nil, &sessions); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids, nil
}

func (m *RealSandboxManager) Wait(ctx context.Context, sandboxID string, timeout time.Duration) (string, string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second
	for time.Now().Before(deadline) {
		status, result, err := m.Status(ctx, sandboxID)
		if err != nil {
			// Transient error — retry.
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}
		if status == "done" || status == "failed" || status == "closed" {
			return status, result, nil
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return "", "", fmt.Errorf("timeout waiting for session %s after %v", sandboxID, timeout)
}

// ---------------------------------------------------------------------------
// RealBusBroker
// ---------------------------------------------------------------------------

// RealBusBroker implements BusBroker by calling the hub's session events
// endpoint for publish and the SSE subscribe endpoint for subscribe.
type RealBusBroker struct {
	client *hubClient
}

func NewRealBusBroker() *RealBusBroker {
	return &RealBusBroker{client: newHubClient()}
}

// Publish sends a message payload to the hub on the given topic.
// For session-scoped topics (session.<id>.*) it posts to the runner events
// endpoint. For other topics it posts to the bus publish endpoint.
func (b *RealBusBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	sessionID := sessionIDFromTopic(topic)
	if sessionID == "" {
		return fmt.Errorf("cannot derive session id from topic %q", topic)
	}
	path := fmt.Sprintf("/api/v1/runners/sessions/%s/events", sessionID)
	resp, err := b.client.do("POST", path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish event: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Subscribe opens an SSE connection to the hub and returns a channel of messages
// on the given topic. The caller must Close the closer when done.
func (b *RealBusBroker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	params := url.Values{"topics": {topic}}
	u := fmt.Sprintf("%s/api/v1/jobs/subscribe?%s", b.client.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("subscribe request: %w", err)
	}
	if b.client.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.client.token)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := b.client.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("subscribe: status %d", resp.StatusCode)
	}

	ch := make(chan []byte, 256)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		br := newSSEBufReader(resp.Body)
		for {
			event, data, ok := br.nextEvent()
			if !ok {
				return
			}
			if event == "message" || event == "" {
				select {
				case ch <- data:
				default:
				}
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return ch, nil
}

// ---------------------------------------------------------------------------
// SSE reader helpers
// ---------------------------------------------------------------------------

// sseBufReader reads SSE-formatted text and extracts events.
type sseBufReader struct {
	rd  io.Reader
	buf []byte
}

func newSSEBufReader(rd io.Reader) *sseBufReader {
	return &sseBufReader{rd: rd, buf: make([]byte, 0, 4096)}
}

// nextEvent reads the next SSE event from the stream. Returns the event type,
// data bytes, and whether more data is available.
func (r *sseBufReader) nextEvent() (event string, data []byte, ok bool) {
	for {
		line, err := r.readLine()
		if err != nil {
			return "", nil, false
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data = append(data, strings.TrimPrefix(line, "data: ")...)
		} else if line == "" {
			return event, data, true
		}
	}
}

func (r *sseBufReader) readLine() (string, error) {
	for i := 0; i < len(r.buf); i++ {
		if r.buf[i] == '\n' {
			line := string(r.buf[:i])
			r.buf = r.buf[i+1:]
			return strings.TrimSuffix(line, "\r"), nil
		}
	}
	tmp := make([]byte, 4096)
	n, err := r.rd.Read(tmp)
	if err != nil {
		if len(r.buf) > 0 {
			line := string(r.buf)
			r.buf = r.buf[:0]
			return line, nil
		}
		return "", err
	}
	r.buf = append(r.buf, tmp[:n]...)
	// Retry after reading more data.
	for i := 0; i < len(r.buf); i++ {
		if r.buf[i] == '\n' {
			line := string(r.buf[:i])
			r.buf = r.buf[i+1:]
			return strings.TrimSuffix(line, "\r"), nil
		}
	}
	// No newline yet — try again with the full buffer as the line.
	if len(r.buf) > 0 {
		line := string(r.buf)
		r.buf = r.buf[:0]
		return line, nil
	}
	return "", io.EOF
}

// sessionIDFromTopic extracts the session ID from a "session.<id>.*" topic.
// Returns "" when the topic is not session-scoped.
func sessionIDFromTopic(topic string) string {
	parts := strings.Split(topic, ".")
	if len(parts) >= 3 && parts[0] == "session" {
		return parts[1]
	}
	return ""
}
