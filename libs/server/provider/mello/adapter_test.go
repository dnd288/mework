package mello

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestVerifyWebhook(t *testing.T) {
	adapter := NewMelloAdapter("")
	secret := "my_signing_secret"
	body := []byte(`{"id":"evt_1","type":"comment.added"}`)

	// Valid signature within timeframe
	nowStr := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nowStr))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	err := adapter.VerifyWebhook(body, nowStr, sig, secret)
	if err != nil {
		t.Fatalf("expected signature to be valid: %v", err)
	}

	// Tampered body
	err = adapter.VerifyWebhook([]byte("tampered"), nowStr, sig, secret)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for tampered body, got: %v", err)
	}

	// Expired timestamp (6 minutes ago)
	sixMinAgoStr := fmt.Sprintf("%d", time.Now().Add(-6*time.Minute).Unix())
	err = adapter.VerifyWebhook(body, sixMinAgoStr, sig, secret)
	if !errors.Is(err, ErrExpiredTimestamp) {
		t.Errorf("expected ErrExpiredTimestamp, got: %v", err)
	}
}

func TestParseEvent(t *testing.T) {
	adapter := NewMelloAdapter("")
	payload := []byte(`{
		"id": "event-123",
		"type": "comment.added",
		"actor": { "id": "user-456", "name": "Alice" },
		"model": { "type": "ticket", "board_id": "board-789" },
		"data": { "id": "comment-abc", "body": "@mework dev review fix the bug", "ticket_id": "ticket-999" }
	}`)

	ev, err := adapter.ParseEvent(payload)
	if err != nil {
		t.Fatalf("ParseEvent failed: %v", err)
	}

	if ev.EventID != "event-123" {
		t.Errorf("expected EventID event-123, got: %s", ev.EventID)
	}
	if ev.EventType != "comment.added" {
		t.Errorf("expected EventType comment.added, got: %s", ev.EventType)
	}
	if ev.Actor.ID != "user-456" || ev.Actor.Name != "Alice" {
		t.Errorf("unexpected actor: %+v", ev.Actor)
	}
	if ev.ExternalTaskID != "ticket-999" {
		t.Errorf("expected ExternalTaskID ticket-999, got: %s", ev.ExternalTaskID)
	}
	if ev.ExternalContainerID != "board-789" {
		t.Errorf("expected ExternalContainerID board-789, got: %s", ev.ExternalContainerID)
	}
	if ev.Body != "@mework dev review fix the bug" {
		t.Errorf("unexpected body: %s", ev.Body)
	}
}

func TestMelloAdapter_ChannelKey(t *testing.T) {
	adapter := NewMelloAdapter("")

	tests := []struct {
		name        string
		payload     []byte
		wantCode    string
		wantResID   string
	}{
		{
			name:        "valid payload with ticket_id",
			payload:     []byte(`{"id":"evt_1","type":"comment.added","data":{"ticket_id":"TICKET-99"}}`),
			wantCode:    "mello",
			wantResID:   "TICKET-99",
		},
		{
			name:        "payload with different ticket_id",
			payload:     []byte(`{"id":"evt_2","type":"comment.added","data":{"ticket_id":"PROJ-42"}}`),
			wantCode:    "mello",
			wantResID:   "PROJ-42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, resID := adapter.ChannelKey(tt.payload)
			if code != tt.wantCode {
				t.Errorf("ChannelKey code = %q, want %q", code, tt.wantCode)
			}
			if resID != tt.wantResID {
				t.Errorf("ChannelKey resourceID = %q, want %q", resID, tt.wantResID)
			}
		})
	}
}

func TestMelloAdapter_ChannelKey_NoTicketID(t *testing.T) {
	adapter := NewMelloAdapter("")

	tests := []struct {
		name      string
		payload   []byte
		wantCode  string
		wantResID string
	}{
		{
			name:      "payload without data field",
			payload:   []byte(`{}`),
			wantCode:  "mello",
			wantResID: "",
		},
		{
			name:      "payload with empty ticket_id",
			payload:   []byte(`{"data":{"ticket_id":""}}`),
			wantCode:  "mello",
			wantResID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, resID := adapter.ChannelKey(tt.payload)
			if code != tt.wantCode {
				t.Errorf("ChannelKey code = %q, want %q", code, tt.wantCode)
			}
			if resID != tt.wantResID {
				t.Errorf("ChannelKey resourceID = %q, want %q", resID, tt.wantResID)
			}
		})
	}
}
