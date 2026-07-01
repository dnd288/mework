package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OutboundPoller polls the mework server for completed Mezon jobs.
type OutboundPoller struct {
	cfg        *Config
	httpClient *http.Client
	cursor     string
}

// doneJob represents a completed job returned by GET /api/v1/jobs.
type doneJob struct {
	ID            string  `json:"id"`
	ProviderCode  string  `json:"provider_code"`
	Status        string  `json:"status"`
	ChannelID     string  `json:"channel_id"`
	Instructions  string  `json:"instructions"`
	ResultSummary *string `json:"result_summary,omitempty"`
	Error         *string `json:"error,omitempty"`
}

// NewOutboundPoller creates a new poller for completed jobs.
func NewOutboundPoller(cfg *Config) *OutboundPoller {
	return &OutboundPoller{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cursor: loadCursor(cfg.CursorDir),
	}
}

// EnqueueJob sends a POST /api/v1/jobs/enqueue to the server.
func (p *OutboundPoller) EnqueueJob(ctx context.Context, channelID, senderID, text, messageID string) {
	payload := map[string]string{
		"provider_code": "mezon",
		"channel_id":    channelID,
		"sender_id":     senderID,
		"text":          text,
		"message_id":    messageID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("enqueue: marshal error: %v", err)
		return
	}

	url := p.cfg.MeworkServerURL + "/api/v1/jobs/enqueue"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("enqueue: request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.MeworkToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("enqueue: http error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("enqueue: server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return
	}

	_, _ = io.Copy(io.Discard, resp.Body)
}

// pollAndProcess polls for done jobs and calls replyFn for each result.
// replyFn receives (channelID, resultText) and should send the reply to Mezon.
func (p *OutboundPoller) pollAndProcess(ctx context.Context, replyFn func(channelID, text string) error) {
	jobs, err := p.pollDoneJobs(ctx)
	if err != nil {
		log.Printf("outbound: poll error: %v", err)
		return
	}

	for _, job := range jobs {
		if job.ResultSummary == nil || *job.ResultSummary == "" {
			p.cursor = job.ID
			saveCursor(p.cfg.CursorDir, job.ID)
			continue
		}

		if err := replyFn(job.ChannelID, *job.ResultSummary); err != nil {
			log.Printf("outbound: reply error for job %s: %v", job.ID, err)
			continue
		}

		p.cursor = job.ID
		saveCursor(p.cfg.CursorDir, job.ID)
		log.Printf("outbound: replied to job %s on channel %s", job.ID, job.ChannelID)
	}
}

func (p *OutboundPoller) pollDoneJobs(ctx context.Context) ([]doneJob, error) {
	url := fmt.Sprintf("%s/api/v1/jobs?provider=mezon&status=done&since=%s",
		p.cfg.MeworkServerURL, p.cursor)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.MeworkToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("poll returned HTTP %d", resp.StatusCode)
	}

	var jobs []doneJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// SendViaSocket sends a message to a local offline agent via Unix socket
// (JSON-RPC "run" method) and returns the agent's response text.
func SendViaSocket(socketPath, instruction string) (string, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("dial socket: %w", err)
	}
	defer conn.Close()

	params, _ := json.Marshal(map[string]string{"instruction": instruction})
	req := jsonRPCRequest{
		Method: "run",
		Params: json.RawMessage(params),
		ID:     1,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}

	var resp jsonRPCResponse
	if err := conn.SetDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		return "", fmt.Errorf("set deadline: %w", err)
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("agent error: %v", resp.Error)
	}

	// Extract output from result.
	if resp.Result != nil {
		if m, ok := resp.Result.(map[string]interface{}); ok {
			if out, ok := m["output"].(string); ok {
				return out, nil
			}
		}
	}
	return "", nil
}

// jsonRPCRequest/Response mirror the offline agent's protocol.
type jsonRPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     interface{}     `json:"id"`
}

type jsonRPCResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
	ID     interface{} `json:"id"`
}

// Cursor persistence.

func cursorPath(dir string) string {
	return filepath.Join(dir, "cursor.txt")
}

func loadCursor(dir string) string {
	data, err := os.ReadFile(cursorPath(dir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveCursor(dir, cursor string) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	_ = os.WriteFile(cursorPath(dir), []byte(cursor), 0600)
}
