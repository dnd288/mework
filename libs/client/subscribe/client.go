package subscribe

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mework/libs/server/bus"
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

// SSEStream wraps an SSE subscription stream, delivering parsed events on a channel.
type SSEStream struct {
	events chan bus.Event
	body   io.ReadCloser
}

// Events returns a channel of parsed SSE events.
func (s *SSEStream) Events() <-chan bus.Event {
	return s.events
}

// Close terminates the SSE subscription and closes the underlying HTTP connection.
func (s *SSEStream) Close() error {
	return s.body.Close()
}

// Subscribe opens an SSE subscription to the given topics. The returned stream
// delivers parsed bus.Event values as they arrive. lastEventID, if non-empty,
// requests resumption from that point.
func (c *Client) Subscribe(rtToken string, topics []string, lastEventID string) (*SSEStream, error) {
	query := url.Values{}
	query.Set("topics", strings.Join(topics, ","))
	if lastEventID != "" {
		query.Set("last_event_id", lastEventID)
	}

	u := c.BaseURL + "/api/v1/jobs/subscribe?" + query.Encode()
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+rtToken)
	req.Header.Set("Accept", "text/event-stream")

	// Use a client with no timeout for long-lived SSE connections.
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("subscribe: %s", string(body))
	}

	events := make(chan bus.Event, 64)
	stream := &SSEStream{
		events: events,
		body:   resp.Body,
	}

	go func() {
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		var id, eventType, data string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "id: "):
				id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				eventType = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "":
				// Empty line marks end of an event frame.
				if id != "" || data != "" {
					ev := bus.Event{
						ID:      id,
						Topic:   bus.Topic(eventType),
						Message: bus.Message{Payload: []byte(data)},
					}
					select {
					case events <- ev:
					default:
					}
				}
				id, eventType, data = "", "", ""
			}
		}
	}()

	return stream, nil
}

// AckMessage posts a delivery acknowledgement for the given message ID.
func (c *Client) AckMessage(rtToken, msgID string) error {
	_, err := c.do("POST", fmt.Sprintf("/api/v1/jobs/messages/%s/ack", msgID), rtToken, nil, nil)
	return err
}
