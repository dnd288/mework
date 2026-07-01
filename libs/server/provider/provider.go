package provider

import (
	"context"
	"sync"
)

type Actor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CanonicalEvent struct {
	EventID             string `json:"event_id"`
	EventType           string `json:"event_type"`
	Actor               Actor  `json:"actor"`
	ExternalTaskID      string `json:"external_task_id"`
	ExternalContainerID string `json:"external_container_id"`
	Body                string `json:"body"`
}

// WebhookHeaderNames declares the HTTP header names a provider uses for
// webhook signature verification, timestamp, and delivery-id fields.
type WebhookHeaderNames struct {
	Signature    string
	Timestamp    string
	DeliveryID   string
}

// TaskDetail holds the platform-specific title and description for a task.
type TaskDetail struct {
	Title       string
	Description string
}

type Provider interface {
	Code() string
	ExtractContainerID(body []byte) (string, error)
	VerifyWebhook(body []byte, timestamp string, signature string, secret string) error
	ParseEvent(payload []byte) (*CanonicalEvent, error)
	WriteBack(ctx context.Context, token string, taskID string, body string) error
	ChannelKey(rawPayload []byte) (providerCode string, resourceID string)

	// WebhookHeaders returns the header names this provider uses for webhook
	// signature verification. The webhook handler calls this instead of
	// hardcoding provider-specific header names.
	WebhookHeaders() WebhookHeaderNames

	// FetchTaskDetail retrieves the platform-specific task title and description
	// for the given external task ID. Returns empty strings when unavailable.
	FetchTaskDetail(ctx context.Context, token, taskID string) (*TaskDetail, error)
}

var (
	providersMu sync.RWMutex
	providers   = make(map[string]Provider)
)

func Register(p Provider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	if p == nil {
		panic("provider: Register provider is nil")
	}
	code := p.Code()
	if _, dup := providers[code]; dup {
		return // Already registered — safe for tests that create multiple servers.
	}
	providers[code] = p
}

func Get(code string) (Provider, bool) {
	providersMu.RLock()
	defer providersMu.RUnlock()
	p, ok := providers[code]
	return p, ok
}
