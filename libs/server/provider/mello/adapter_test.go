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
