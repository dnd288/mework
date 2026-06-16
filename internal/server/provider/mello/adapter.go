package mello

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mework/internal/mello"
	"mework/internal/server/provider"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrExpiredTimestamp = errors.New("webhook timestamp expired or too far in future")
)

type MelloAdapter struct {
	melloBaseURL string
}

func NewMelloAdapter(melloBaseURL string) *MelloAdapter {
	return &MelloAdapter{
		melloBaseURL: melloBaseURL,
	}
}

func (a *MelloAdapter) Code() string {
	return "mello"
}

func (a *MelloAdapter) ExtractContainerID(body []byte) (string, error) {
	var p struct {
		Model struct {
			BoardID string `json:"board_id"`
		} `json:"model"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", fmt.Errorf("failed to pre-parse board_id: %w", err)
	}
	return p.Model.BoardID, nil
}

func (a *MelloAdapter) VerifyWebhook(body []byte, timestamp string, signature string, secret string) error {
	if secret == "" {
		return errors.New("empty webhook secret")
	}

	// 1. Replay protection: ±5 min window
	tsInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	ts := time.Unix(tsInt, 0)
	now := time.Now()
	diff := now.Sub(ts)
	if diff < -5*time.Minute || diff > 5*time.Minute {
		return ErrExpiredTimestamp
	}

	// 2. Signature verification: HMAC-SHA256(secret, timestamp + "." + body)
	if strings.HasPrefix(signature, "sha256=") {
		signature = signature[len("sha256="):]
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedMAC), []byte(signature)) {
		return ErrInvalidSignature
	}

	return nil
}

type melloWebhookPayload struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Actor struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"actor"`
	Model struct {
		Type    string `json:"type"`
		BoardID string `json:"board_id"`
	} `json:"model"`
	Data struct {
		ID       string `json:"id"`
		Body     string `json:"body"`
		TicketID string `json:"ticket_id"`
	} `json:"data"`
}

func (a *MelloAdapter) ParseEvent(payload []byte) (*provider.CanonicalEvent, error) {
	var p melloWebhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook payload: %w", err)
	}

	if p.Type != "comment.added" {
		return nil, fmt.Errorf("unsupported event type: %s", p.Type)
	}

	return &provider.CanonicalEvent{
		EventID:             p.ID,
		EventType:           p.Type,
		Actor:               provider.Actor{ID: p.Actor.ID, Name: p.Actor.Name},
		ExternalTaskID:      p.Data.TicketID,
		ExternalContainerID: p.Model.BoardID,
		Body:                p.Data.Body,
	}, nil
}

func (a *MelloAdapter) WriteBack(ctx context.Context, token string, taskID string, body string) error {
	client := mello.NewClient(a.melloBaseURL, token, 30*time.Second, "mework-server")
	_, err := client.CreateComment(taskID, body)
	return err
}
