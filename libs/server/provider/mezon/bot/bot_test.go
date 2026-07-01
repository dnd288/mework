// Package bot tests the Mezon bot client wrapper — a thin wrapper around the
// mezon-go-sdk that adapts the SDK client (auth, WebSocket, protobuf, heartbeat,
// reconnection) to mework's internal types and dispatch model.
//
// RED step: all tests fail to compile because the production code
// (Bot, New, Message, MessageHandler, SDKClient interface) does not exist yet.
package bot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	sharedMezon "mework/libs/shared/providers/mezon"
)

// ---------------------------------------------------------------------------
// Mock SDK client
// ---------------------------------------------------------------------------

// mockSDKMessage simulates the raw message type the SDK would pass to the
// OnMessage callback registered by the Bot.
type mockSDKMessage struct {
	ChannelID string
	SenderID  string
	Text      string
}

// mockSDKClient implements the SDKClient interface (defined in the production
// bot.go) for testing. All exported methods are safe for concurrent access.
type mockSDKClient struct {
	mu sync.Mutex

	// Auth configuration and tracking
	authToken     string
	authUserID    string
	authErr       error
	authCallCount int

	// Connect configuration and tracking
	connectErr  error
	connected   bool

	// Callbacks registered by the Bot
	onMessageFn  func(interface{})
	onReconnectFn func()

	// Send tracking
	lastChannelID string
	lastText      string
	sendErr       error

	// Close tracking
	closeCalled bool
	closeErr    error
}

func (m *mockSDKClient) Authenticate() (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authCallCount++
	return m.authToken, m.authUserID, m.authErr
}

func (m *mockSDKClient) OnMessage(fn func(interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMessageFn = fn
}

func (m *mockSDKClient) OnReconnect(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReconnectFn = fn
}

func (m *mockSDKClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return m.connectErr
}

func (m *mockSDKClient) SendText(channelID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastChannelID = channelID
	m.lastText = text
	return m.sendErr
}

func (m *mockSDKClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	m.connected = false
	return m.closeErr
}

// triggerMessage invokes the OnMessage callback registered by the Bot, if any.
func (m *mockSDKClient) triggerMessage(msg interface{}) {
	m.mu.Lock()
	fn := m.onMessageFn
	m.mu.Unlock()
	if fn != nil {
		fn(msg)
	}
}

// triggerReconnect invokes the OnReconnect callback registered by the Bot, if any.
func (m *mockSDKClient) triggerReconnect() {
	m.mu.Lock()
	fn := m.onReconnectFn
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// defaultMockClient returns a mockSDKClient configured for success on every
// operation — individual tests override the fields they need.
func defaultMockClient() *mockSDKClient {
	return &mockSDKClient{
		authToken:  "sdk-test-token-abc",
		authUserID: "bot-user-456",
	}
}

// ---------------------------------------------------------------------------
// Bot tests
// ---------------------------------------------------------------------------

func TestBot_Authenticate_Success(t *testing.T) {
	mock := defaultMockClient()
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {})

	err := bot.Authenticate()
	if err != nil {
		t.Fatalf("Authenticate() returned error: %v", err)
	}
	if bot.botUserID != "bot-user-456" {
		t.Errorf("botUserID = %q, want %q", bot.botUserID, "bot-user-456")
	}
	if mock.authCallCount != 1 {
		t.Errorf("Authenticate called %d time(s), want 1", mock.authCallCount)
	}
}

func TestBot_Authenticate_Failure(t *testing.T) {
	tests := []struct {
		name    string
		authErr error
	}{
		{name: "invalid credentials", authErr: errors.New("invalid app_id or api_key")},
		{name: "network error", authErr: errors.New("connection refused")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := defaultMockClient()
			mock.authErr = tt.authErr
			bot := New(sharedMezon.Config{
				AppID:  "test-app",
				APIKey: "test-key",
			}, mock, func(msg Message) {})

			err := bot.Authenticate()
			if err == nil {
				t.Fatal("Authenticate() expected error, got nil")
			}
			// No WebSocket connection should be attempted when auth fails.
			if mock.connected {
				t.Error("Connect() was called despite authentication failure")
			}
		})
	}
}

func TestBot_Connect_Success(t *testing.T) {
	mock := defaultMockClient()
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {})

	// Authenticate first, then connect.
	if err := bot.Authenticate(); err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}

	err := bot.Connect()
	if err != nil {
		t.Fatalf("Connect() returned error: %v", err)
	}
	if !bot.IsConnected() {
		t.Error("IsConnected() = false, want true after successful Connect()")
	}
}

func TestBot_Connect_Reauthenticate(t *testing.T) {
	tests := []struct {
		name         string
		firstAuthErr error
		secondAuthOk bool
		expectFinal  bool // whether the bot ends up connected
	}{
		{
			name:         "auth retry succeeds",
			firstAuthErr: errors.New("session expired"),
			secondAuthOk: true,
			expectFinal:  true,
		},
		{
			name:         "auth retry fails",
			firstAuthErr: errors.New("invalid credentials"),
			secondAuthOk: false,
			expectFinal:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := defaultMockClient()
			// Make the mock fail auth the first time.
			mock.authErr = tt.firstAuthErr
			bot := New(sharedMezon.Config{
				AppID:  "test-app",
				APIKey: "test-key",
			}, mock, func(msg Message) {})

			// First attempt should fail.
			if err := bot.Authenticate(); err == nil {
				t.Fatal("expected first Authenticate() to fail")
			}

			// Update the mock for the retry.
			if tt.secondAuthOk {
				mock.mu.Lock()
				mock.authErr = nil
				mock.mu.Unlock()
			}

			// The bot's Connect (or the bot's reconnection logic) re-authenticates.
			// We simulate this by calling Authenticate + Connect again.
			err := bot.Authenticate()
			if tt.secondAuthOk && err != nil {
				t.Fatalf("second Authenticate() failed: %v", err)
			}
			if !tt.secondAuthOk && err == nil {
				t.Fatal("expected second Authenticate() to fail")
			}
			if !tt.secondAuthOk {
				return // auth failed, don't proceed to connect
			}

			if bot.botUserID != "bot-user-456" {
				t.Errorf("botUserID = %q, want %q", bot.botUserID, "bot-user-456")
			}
			if mock.authCallCount != 2 {
				t.Errorf("Authenticate called %d time(s), want 2", mock.authCallCount)
			}

			// Now connect should work.
			if err := bot.Connect(); err != nil {
				t.Fatalf("Connect() after re-auth failed: %v", err)
			}
			if !bot.IsConnected() != !tt.expectFinal {
				t.Errorf("IsConnected() = %v, want %v", bot.IsConnected(), tt.expectFinal)
			}
		})
	}
}

func TestBot_ReceiveMessage(t *testing.T) {
	// Arrange: create a bot that captures received messages.
	received := make(chan Message, 1)
	mock := defaultMockClient()
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {
		received <- msg
	})

	// Start registers the OnMessage callback.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = bot.Start(ctx) // Start blocks until context cancelled; expected.
	}()

	// Wait for Start to register callbacks.
	time.Sleep(50 * time.Millisecond)

	// Act: simulate receiving a message from the SDK.
	mock.triggerMessage(mockSDKMessage{
		ChannelID: "ch_abc123",
		SenderID:  "user-789",
		Text:      "hello from mezon",
	})

	// Assert: the bot's dispatch callback receives the adapted Message.
	select {
	case msg := <-received:
		if msg.ChannelID != "ch_abc123" {
			t.Errorf("ChannelID = %q, want %q", msg.ChannelID, "ch_abc123")
		}
		if msg.SenderID != "user-789" {
			t.Errorf("SenderID = %q, want %q", msg.SenderID, "user-789")
		}
		if msg.Text != "hello from mezon" {
			t.Errorf("Text = %q, want %q", msg.Text, "hello from mezon")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message dispatch")
	}
}

func TestBot_SelfMessageFiltered(t *testing.T) {
	// Arrange: create a bot that flags if a message is dispatched.
	selfDispatched := make(chan struct{}, 1)
	mock := defaultMockClient()
	mock.authUserID = "bot-self-999"
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {
		selfDispatched <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = bot.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Act: authenticate (so botUserID is set) then simulate a self-authored message.
	_ = bot.Authenticate() // sets botUserID = "bot-self-999"

	mock.triggerMessage(mockSDKMessage{
		ChannelID: "ch_abc",
		SenderID:  "bot-self-999", // matches bot's own user ID
		Text:      "I wrote this",
	})

	// Assert: the dispatch callback must NOT be called.
	select {
	case <-selfDispatched:
		t.Fatal("self-authored message was dispatched; should have been filtered")
	case <-time.After(500 * time.Millisecond):
		// Expected: no dispatch within the window.
	}
}

func TestBot_SendMessage(t *testing.T) {
	mock := defaultMockClient()
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {})

	err := bot.SendMessage(context.Background(), "ch_send_001", "Hello, Mezon channel!")
	if err != nil {
		t.Fatalf("SendMessage() returned error: %v", err)
	}
	if mock.lastChannelID != "ch_send_001" {
		t.Errorf("channelID = %q, want %q", mock.lastChannelID, "ch_send_001")
	}
	if mock.lastText != "Hello, Mezon channel!" {
		t.Errorf("text = %q, want %q", mock.lastText, "Hello, Mezon channel!")
	}
}

func TestBot_SendMessage_Error(t *testing.T) {
	tests := []struct {
		name    string
		sendErr error
	}{
		{name: "network error", sendErr: errors.New("websocket disconnected")},
		{name: "rate limited", sendErr: errors.New("rate limit exceeded")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := defaultMockClient()
			mock.sendErr = tt.sendErr
			bot := New(sharedMezon.Config{
				AppID:  "test-app",
				APIKey: "test-key",
			}, mock, func(msg Message) {})

			err := bot.SendMessage(context.Background(), "ch_err", "oops")
			if err == nil {
				t.Fatal("SendMessage() expected error, got nil")
			}
		})
	}
}

func TestBot_Reconnect(t *testing.T) {
	// Arrange: create a bot and connect.
	mock := defaultMockClient()
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {})

	if err := bot.Authenticate(); err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}
	if err := bot.Connect(); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	initialAuthCalls := mock.authCallCount

	// Act: simulate a reconnection event from the SDK.
	mock.triggerReconnect()

	// Assert: the bot re-authenticated in response to the reconnection.
	if mock.authCallCount <= initialAuthCalls {
		t.Error("bot did not re-authenticate after SDK reconnect event")
	}
	if !bot.IsConnected() {
		t.Error("IsConnected() = false after reconnect handling")
	}

	// After reconnection, message dispatch should still work.
	received := make(chan Message, 1)
	bot.handler = func(msg Message) {
		received <- msg
	}
	mock.triggerMessage(mockSDKMessage{
		ChannelID: "ch_reconnect",
		SenderID:  "user-789",
		Text:      "message after reconnect",
	})
	select {
	case msg := <-received:
		if msg.ChannelID != "ch_reconnect" {
			t.Errorf("ChannelID = %q, want %q", msg.ChannelID, "ch_reconnect")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message dispatch after reconnect")
	}
}

func TestBot_Shutdown(t *testing.T) {
	mock := defaultMockClient()
	bot := New(sharedMezon.Config{
		AppID:   "test-app",
		APIKey:  "test-key",
		BaseURL: "https://test.mezon.example",
	}, mock, func(msg Message) {})

	// Start the bot in a background goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- bot.Start(ctx)
	}()

	// Let the bot fully initialise.
	time.Sleep(50 * time.Millisecond)

	// Act: cancel the context and wait for Stop.
	cancel()

	select {
	case err := <-startErrCh:
		if err != nil {
			t.Errorf("Start() returned error during shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Start() to return after context cancellation")
	}

	if err := bot.Stop(); err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
	if bot.IsConnected() {
		t.Error("IsConnected() = true after Stop(), want false")
	}
	if !mock.closeCalled {
		t.Error("SDK client Close() was not called during shutdown")
	}
}
