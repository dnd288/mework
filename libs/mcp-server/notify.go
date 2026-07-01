// Package main implements the mework MCP server binary.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// NotifyHandler implements MCP tool handlers for notification / human-interaction
// tools: notify_human and ask_human.
type NotifyHandler struct {
	bus BusBroker
}

// NewNotifyHandler creates a new NotifyHandler backed by the given message bus.
func NewNotifyHandler(bus BusBroker) *NotifyHandler {
	return &NotifyHandler{bus: bus}
}

// sessionID returns the MEWORK_SESSION_ID environment variable value.
// Empty string means no session context is available.
func (h *NotifyHandler) sessionID() string {
	return os.Getenv("MEWORK_SESSION_ID")
}

// notificationPayload is the JSON payload published for notify_human.
type notificationPayload struct {
	Message     string   `json:"message"`
	Format      string   `json:"format,omitempty"`
	Attachments []string `json:"attachments,omitempty"`
}

// NotifyHuman handles the notify_human MCP tool.
//
// Parameters:
//   - message (string, required): the message to send
//   - format (string, optional): "text" or "markdown"
//   - attachments ([]string, optional): file attachment paths
//
// When MEWORK_SESSION_ID is set, the message is published on the
// session.<id>.output topic. When unset, the handler falls back to
// stdout logging (no crash, no bus publish).
func (h *NotifyHandler) NotifyHuman(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	msg, _ := args["message"].(string)

	sessionID := h.sessionID()
	if sessionID == "" {
		log.Printf("[notify_human] %s", msg)
		return map[string]string{"status": "logged"}, nil
	}

	format, _ := args["format"].(string)

	notification := notificationPayload{
		Message: msg,
		Format:  format,
	}

	if attsRaw, ok := args["attachments"].([]interface{}); ok {
		atts := make([]string, 0, len(attsRaw))
		for _, a := range attsRaw {
			if s, ok := a.(string); ok {
				atts = append(atts, s)
			}
		}
		notification.Attachments = atts
	}

	payload, err := json.Marshal(notification)
	if err != nil {
		return nil, fmt.Errorf("marshal notification: %w", err)
	}

	topic := fmt.Sprintf("session.%s.output", sessionID)
	if err := h.bus.Publish(ctx, topic, payload); err != nil {
		return nil, fmt.Errorf("publish notification: %w", err)
	}

	return map[string]string{"status": "sent"}, nil
}

// questionPayload is the JSON payload published for ask_human.
type questionPayload struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

// responsePayload is the JSON structure expected for human responses.
type responsePayload struct {
	Response string `json:"response"`
}

// AskHuman handles the ask_human MCP tool.
//
// Parameters:
//   - question (string, required): the question to ask
//   - options ([]string, optional): valid response choices
//   - timeout_minutes (float64, optional, default 5): max wait time
//
// The question is published on the session.<id>.output topic and the
// handler subscribes to the .input to receive the human's response.
// When options are provided the response is validated against the list.
// When MEWORK_SESSION_ID is not set the handler returns an error.
func (h *NotifyHandler) AskHuman(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	question, _ := args["question"].(string)

	sessionID := h.sessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("no session context: MEWORK_SESSION_ID not set")
	}

	// Parse optional options list.
	var options []string
	if optsRaw, ok := args["options"].([]interface{}); ok {
		for _, o := range optsRaw {
			if s, ok := o.(string); ok {
				options = append(options, s)
			}
		}
	}

	// Parse timeout (default 5 minutes).
	timeoutMinutes := 5.0
	if tm, ok := args["timeout_minutes"].(float64); ok && tm > 0 {
		timeoutMinutes = tm
	}

	outputTopic := fmt.Sprintf("session.%s.output", sessionID)
	inputTopic := fmt.Sprintf("session.%s.input", sessionID)

	// Publish the question on the output topic so the human client sees it.
	qMsg := questionPayload{
		Question: question,
		Options:  options,
	}
	qPayload, err := json.Marshal(qMsg)
	if err != nil {
		return nil, fmt.Errorf("marshal question: %w", err)
	}
	if err := h.bus.Publish(ctx, outputTopic, qPayload); err != nil {
		return nil, fmt.Errorf("publish question: %w", err)
	}

	// Subscribe to the input topic to wait for the human's response.
	subCh, err := h.bus.Subscribe(ctx, inputTopic)
	if err != nil {
		return nil, fmt.Errorf("subscribe to input topic: %w", err)
	}

	timeout := time.Duration(timeoutMinutes * float64(time.Minute))
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Read from the subscription until a valid response arrives.
	for {
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("timeout waiting for human response")
		case msg := <-subCh:
			var resp responsePayload
			if err := json.Unmarshal(msg, &resp); err != nil {
				continue
			}
			if resp.Response == "" {
				continue
			}
			if len(options) > 0 {
				valid := false
				for _, opt := range options {
					if resp.Response == opt {
						valid = true
						break
					}
				}
				if !valid {
					continue
				}
			}
			return map[string]string{"response": resp.Response}, nil
		}
	}
}
