package provider

import (
	"context"
	"encoding/json"
	"testing"
)

type mockProvider struct {
	code string
}

func (m *mockProvider) Code() string {
	return m.code
}

func (m *mockProvider) ExtractContainerID(body []byte) (string, error) {
	return "mock_container", nil
}

func (m *mockProvider) VerifyWebhook(body []byte, timestamp string, signature string, secret string) error {
	return nil
}

func (m *mockProvider) ParseEvent(payload []byte) (*CanonicalEvent, error) {
	return nil, nil
}

func (m *mockProvider) WriteBack(ctx context.Context, token string, taskID string, body string) error {
	return nil
}

func (m *mockProvider) ChannelKey(rawPayload []byte) (string, string) {
	return m.code, "mock-resource"
}

// melloKeyMock extracts ticket_id from a Mello-style webhook payload.
// Used by TestProviderInterface_HasChannelKey to verify the interface
// contract without importing the mello adapter (which would create a cycle).
type melloKeyMock struct{ code string }

func (m *melloKeyMock) Code() string                       { return m.code }
func (m *melloKeyMock) ExtractContainerID([]byte) (string, error) { return "", nil }
func (m *melloKeyMock) VerifyWebhook([]byte, string, string, string) error { return nil }
func (m *melloKeyMock) ParseEvent([]byte) (*CanonicalEvent, error) { return nil, nil }
func (m *melloKeyMock) WriteBack(context.Context, string, string, string) error { return nil }
func (m *melloKeyMock) ChannelKey(raw []byte) (string, string) {
	var p struct {
		Data struct {
			TicketID string `json:"ticket_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Data.TicketID == "" {
		return "", ""
	}
	return m.code, p.Data.TicketID
}

func TestProviderRegistry(t *testing.T) {
	p := &mockProvider{code: "mock_provider"}

	// Before registration
	if _, ok := Get("mock_provider"); ok {
		t.Fatal("expected mock_provider not to be registered")
	}

	Register(p)

	// After registration
	got, ok := Get("mock_provider")
	if !ok {
		t.Fatal("expected mock_provider to be registered")
	}

	if got != p {
		t.Error("expected retrieved provider to match registered provider")
	}
}

// TestProviderInterface_HasChannelKey verifies that the Provider interface
// includes the ChannelKey method. This test fails to compile in RED because
// Provider does not yet expose ChannelKey.
func TestProviderInterface_HasChannelKey(t *testing.T) {
	tests := []struct {
		name       string
		payload    []byte
		wantCode   string
		wantResID  string
	}{
		{
			name:       "mello payload returns code and resource ID",
			payload:    []byte(`{"data":{"ticket_id":"TICKET-99"}}`),
			wantCode:   "mello",
			wantResID:  "TICKET-99",
		},
		{
			name:       "empty payload returns empty strings",
			payload:    []byte(`{}`),
			wantCode:   "",
			wantResID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Provider = &melloKeyMock{code: "mello"}
			// This line fails to compile until ChannelKey is added to Provider:
			code, resID := p.ChannelKey(tt.payload)
			if code != tt.wantCode {
				t.Errorf("ChannelKey code = %q, want %q", code, tt.wantCode)
			}
			if resID != tt.wantResID {
				t.Errorf("ChannelKey resourceID = %q, want %q", resID, tt.wantResID)
			}
		})
	}
}

// TestProviderInterface_GitHubMock demonstrates the provider-agnostic contract
// with a GitHub adapter stub, verifying ChannelKey works identically.
func TestProviderInterface_GitHubMock(t *testing.T) {
	gh := &mockProvider{code: "github"}
	var p Provider = gh
	code, resID := p.ChannelKey([]byte(`{"issue":{"number":42}}`))
	if code != "github" {
		t.Errorf("ChannelKey code = %q, want %q", code, "github")
	}
	if resID == "" {
		t.Error("ChannelKey resourceID should not be empty for valid payload")
	}
}

// --- Unit 03: Write-back contract assertions ---

// TestProvider_ChannelKeyToWriteBack verifies the complete contract: ChannelKey
// resolves a provider code and resource ID, and WriteBack accepts those values
// to post a result. This ensures the interface supports the channel-session
// writeback flow end-to-end.
func TestProvider_ChannelKeyToWriteBack(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		payload  []byte
		wantCode string
		wantBody string
	}{
		{
			name:     "mello channel key resolves to WriteBack",
			provider: &mockProvider{code: "mello"},
			payload:  []byte(`{"data":{"ticket_id":"TICKET-99"}}`),
			wantCode: "mello",
			wantBody: "review complete",
		},
		{
			name:     "github channel key resolves to WriteBack",
			provider: &mockProvider{code: "github"},
			payload:  []byte(`{"issue":{"number":42}}`),
			wantCode: "github",
			wantBody: "fix applied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, resID := tt.provider.ChannelKey(tt.payload)
			if code != tt.wantCode {
				t.Fatalf("ChannelKey code = %q, want %q", code, tt.wantCode)
			}
			if resID == "" {
				t.Fatal("ChannelKey resourceID should be non-empty for valid payload")
			}

			// The write-back path: provider code + resource ID -> WriteBack.
			err := tt.provider.WriteBack(context.Background(), "test-token", resID, tt.wantBody)
			if err != nil {
				t.Fatalf("WriteBack after ChannelKey failed: %v", err)
			}
		})
	}
}

// TestProvider_ChannelKey_WriteBack_InterfaceContract is a compile-time check
// that ChannelKey returns (code, resourceID) — values that are valid inputs to
// the Provider registry lookup and WriteBack.
func TestProvider_ChannelKey_WriteBack_InterfaceContract(t *testing.T) {
	var p Provider = &mockProvider{code: "mello"}
	code, resourceID := p.ChannelKey([]byte(`{"data":{"ticket_id":"TICKET-99"}}`))

	// Verify that the code from ChannelKey can be used to look up the provider
	// and call WriteBack as the write-back pipeline would.
	got, ok := Get(code)
	if !ok {
		t.Fatalf("Get(%q) after ChannelKey: provider not found", code)
	}
	if got.Code() != code {
		t.Errorf("resolved provider code = %q, want %q", got.Code(), code)
	}
	if err := got.WriteBack(context.Background(), "test-token", resourceID, "done"); err != nil {
		t.Errorf("WriteBack with ChannelKey output failed: %v", err)
	}
}
