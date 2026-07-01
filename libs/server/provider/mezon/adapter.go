package mezon

import (
	"context"
	"encoding/json"
	"fmt"

	"mework/libs/server/provider"
)

// BotSender is the interface for sending messages to a Mezon channel.
// The adapter delegates write-back to this interface; the bot handles
// WebSocket vs REST internally.
type BotSender interface {
	SendMessage(ctx context.Context, channelID, text string) error
}

// MezonAdapter implements the provider.Provider interface for the Mezon platform.
// It provides channel key extraction, event parsing, write-back via the bot, and
// empty/no-op implementations for webhook and task methods that are not applicable
// to Mezon's WebSocket-based communication model.
type MezonAdapter struct {
	bot BotSender
}

// NewMezonAdapter creates a new MezonAdapter wrapping the given bot sender.
func NewMezonAdapter(bot BotSender) *MezonAdapter {
	return &MezonAdapter{bot: bot}
}

// Code returns the provider identifier.
func (a *MezonAdapter) Code() string { return "mezon" }

// ExtractContainerID returns an empty container ID (not applicable for Mezon).
func (a *MezonAdapter) ExtractContainerID(body []byte) (string, error) {
	return "", nil
}

// VerifyWebhook is a no-op — Mezon uses WebSocket, not webhooks.
func (a *MezonAdapter) VerifyWebhook(body []byte, timestamp, signature, secret string) error {
	return nil
}

// ParseEvent parses a Mezon channel message into a canonical event.
func (a *MezonAdapter) ParseEvent(payload []byte) (*provider.CanonicalEvent, error) {
	var msg struct {
		ChannelID string `json:"channel_id"`
		SenderID  string `json:"sender_id"`
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mezon message: %w", err)
	}
	return &provider.CanonicalEvent{
		EventID:   msg.MessageID,
		EventType: "message.created",
		Actor:     provider.Actor{ID: msg.SenderID},
		Body:      msg.Text,
	}, nil
}

// WriteBack sends a reply message to the given channel.
// taskID is the channel ID for Mezon (channel messages don't have tasks).
func (a *MezonAdapter) WriteBack(ctx context.Context, token, taskID, body string) error {
	return a.bot.SendMessage(ctx, taskID, body)
}

// ChannelKey extracts ("mezon", channelID) from the raw payload.
func (a *MezonAdapter) ChannelKey(rawPayload []byte) (string, string) {
	var p struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(rawPayload, &p); err != nil {
		return "mezon", ""
	}
	return "mezon", p.ChannelID
}

// WebhookHeaders returns empty headers — Mezon has no webhook verification.
func (a *MezonAdapter) WebhookHeaders() provider.WebhookHeaderNames {
	return provider.WebhookHeaderNames{}
}

// FetchTaskDetail returns an empty TaskDetail — Mezon has no task concept.
func (a *MezonAdapter) FetchTaskDetail(ctx context.Context, token, taskID string) (*provider.TaskDetail, error) {
	return &provider.TaskDetail{}, nil
}
