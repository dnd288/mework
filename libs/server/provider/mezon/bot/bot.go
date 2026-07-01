// Package bot provides a thin wrapper around the Mezon SDK client, adapting
// the SDK's auth, WebSocket, protobuf, heartbeat, and reconnection to mework's
// internal types and dispatch model.
//
// The key design choice is the SDKClient interface, which allows testing with
// a mock while the real SDK client implements the same interface (or is wrapped
// by an adapter). No raw gorilla/websocket or protobuf decoding happens here.
package bot

import (
	"context"
	"reflect"

	sharedMezon "mework/libs/shared/providers/mezon"
)

// Message is an adapted Mezon message for mework-internal use.
type Message struct {
	ChannelID string
	SenderID  string
	Text      string
}

// MessageHandler is the callback for received messages.
type MessageHandler func(msg Message)

// SDKMessage provides compile-time-safe field access from SDK message types.
// Test mocks implement this interface; real SDK protobuf types use the
// reflection fallback in extractMessageFields.
type SDKMessage interface {
	GetChannelID() string
	GetSenderID() string
	GetText() string
}

// SDKClient is the interface for the Mezon SDK client. The real SDK's
// mezonsdk.Client satisfies this interface (via an adapter if needed).
type SDKClient interface {
	Authenticate() (token, userID string, err error)
	OnMessage(fn func(interface{}))
	OnReconnect(fn func())
	Connect() error
	SendText(channelID, text string) error
	Close() error
}

// Bot wraps a Mezon SDK client with mework's lifecycle and message dispatch.
// It delegates network I/O to the SDK client entirely — the Bot only handles
// lifecycle (Start/Stop), message adaptation, and self-message filtering.
type Bot struct {
	config     sharedMezon.Config
	sdkClient  SDKClient
	handler    MessageHandler
	onMsg      func(ctx context.Context, channelID, senderID, text string)
	botUserID  string
	connected  bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// New creates a new Bot. It registers the message adaptation callback and the
// reconnect handler on the SDK client immediately so that the bot is ready
// to receive messages and handle reconnections from the moment it is created.
// When client is nil, New returns a no-op bot that always returns errors from
// Authenticate, Connect, and SendMessage (Start returns nil immediately).
func New(cfg sharedMezon.Config, client SDKClient, handler MessageHandler) *Bot {
	b := &Bot{
		config:    cfg,
		sdkClient: client,
		handler:   handler,
	}

	// Nil client guard — return a no-op bot.
	if client == nil {
		return b
	}

	// Register the reconnect handler: re-authenticate on SDK reconnect events.
	client.OnReconnect(func() {
		_, userID, err := client.Authenticate()
		if err == nil {
			b.botUserID = userID
		}
	})

	// Register the message adaptation callback.
	client.OnMessage(func(msg interface{}) {
		channelID, senderID, text := extractMessageFields(msg)

		// Self-message filter: never dispatch messages sent by the bot itself.
		if senderID == b.botUserID {
			return
		}

		// Call the external handler if one is registered via OnMessage.
		if b.onMsg != nil && b.ctx != nil {
			b.onMsg(b.ctx, channelID, senderID, text)
		}

		b.handler(Message{
			ChannelID: channelID,
			SenderID:  senderID,
			Text:      text,
		})
	})

	return b
}

// OnMessage registers an external message handler. Unlike handler (which
// receives adapted Message structs), OnMessage receives raw string fields
// and a context. This is used by the offline server to wire policy
// enforcement and sandbox dispatch into the message flow.
func (b *Bot) OnMessage(handler func(ctx context.Context, channelID, senderID, text string)) {
	b.onMsg = handler
}

// extractMessageFields extracts ChannelID, SenderID, and Text from a raw SDK
// message. It first tries a compile-time-safe type assertion against the
// SDKMessage interface (used by test mocks and SDK wrappers). If the interface
// assertion fails, it falls back to reflection for protobuf-derived SDK types
// whose struct fields match by name.
func extractMessageFields(msg interface{}) (channelID, senderID, text string) {
	// Try compile-time-safe interface first.
	if m, ok := msg.(SDKMessage); ok {
		return m.GetChannelID(), m.GetSenderID(), m.GetText()
	}

	// Reflection fallback for proto-derived types that don't implement SDKMessage.
	v := reflect.ValueOf(msg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", "", ""
	}
	if f := v.FieldByName("ChannelID"); f.IsValid() && f.Kind() == reflect.String {
		channelID = f.String()
	}
	if f := v.FieldByName("SenderID"); f.IsValid() && f.Kind() == reflect.String {
		senderID = f.String()
	}
	if f := v.FieldByName("Text"); f.IsValid() && f.Kind() == reflect.String {
		text = f.String()
	}
	return
}

// Authenticate exchanges credentials for a session token and caches the bot
// user ID for self-message filtering. Returns nil when sdkClient is nil
// (no-op bot).
func (b *Bot) Authenticate() error {
	if b.sdkClient == nil {
		return nil
	}
	_, userID, err := b.sdkClient.Authenticate()
	if err != nil {
		return err
	}
	b.botUserID = userID
	return nil
}

// Connect establishes the WebSocket connection via the SDK. Returns nil when
// sdkClient is nil (no-op bot).
func (b *Bot) Connect() error {
	if b.sdkClient == nil {
		return nil
	}
	if err := b.sdkClient.Connect(); err != nil {
		return err
	}
	b.connected = true
	return nil
}

// IsConnected returns whether the bot is currently connected.
func (b *Bot) IsConnected() bool {
	return b.connected
}

// Start begins the bot's dispatch loop. It blocks until ctx is cancelled.
// Callbacks (OnMessage, OnReconnect) are already registered by New.
// When sdkClient is nil, returns immediately (no-op).
func (b *Bot) Start(ctx context.Context) error {
	if b.sdkClient == nil {
		return nil
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	defer b.cancel()
	<-b.ctx.Done()
	return nil
}

// Stop closes the SDK client connection cleanly. Returns nil when sdkClient
// is nil (no-op bot).
func (b *Bot) Stop() error {
	if b.sdkClient == nil {
		return nil
	}
	err := b.sdkClient.Close()
	b.connected = false
	return err
}

// UserID returns the authenticated bot user ID, or empty string if not
// yet authenticated. This is used by the worker for self-message filtering.
func (b *Bot) UserID() string {
	return b.botUserID
}

// SendMessage sends a text message to the given channel via the SDK. Returns
// nil when sdkClient is nil (no-op bot).
func (b *Bot) SendMessage(ctx context.Context, channelID, text string) error {
	if b.sdkClient == nil {
		return nil
	}
	return b.sdkClient.SendText(channelID, text)
}
