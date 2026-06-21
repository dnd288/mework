package provider

import (
	"context"
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
