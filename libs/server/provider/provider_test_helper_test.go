package provider

import "context"

// testMockProvider is a minimal Provider implementation used to register
// well-known provider codes during test builds. This allows tests like
// TestProvider_ChannelKey_WriteBack_InterfaceContract to verify the
// ChannelKey → Get → WriteBack pipeline without requiring real adapters.
type testMockProvider struct {
	code string
}

func (m *testMockProvider) Code() string                                    { return m.code }
func (m *testMockProvider) ExtractContainerID([]byte) (string, error)       { return "", nil }
func (m *testMockProvider) VerifyWebhook([]byte, string, string, string) error { return nil }
func (m *testMockProvider) ParseEvent([]byte) (*CanonicalEvent, error)      { return nil, nil }
func (m *testMockProvider) WriteBack(context.Context, string, string, string) error { return nil }
func (m *testMockProvider) WebhookHeaders() WebhookHeaderNames {
	return WebhookHeaderNames{
		Signature:  "X-Signature",
		Timestamp:  "X-Timestamp",
		DeliveryID: "X-Delivery",
	}
}
func (m *testMockProvider) FetchTaskDetail(_ context.Context, _, _ string) (*TaskDetail, error) {
	return nil, nil
}
func (m *testMockProvider) ChannelKey([]byte) (string, string) { return m.code, "mock-resource" }

func init() {
	// Register well-known provider codes so Get("mello") and Get("github")
	// succeed during test builds.
	Register(&testMockProvider{code: "mello"})
	Register(&testMockProvider{code: "github"})
}
