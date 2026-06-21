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

type Provider interface {
	Code() string
	ExtractContainerID(body []byte) (string, error)
	VerifyWebhook(body []byte, timestamp string, signature string, secret string) error
	ParseEvent(payload []byte) (*CanonicalEvent, error)
	WriteBack(ctx context.Context, token string, taskID string, body string) error
	ChannelKey(rawPayload []byte) (providerCode string, resourceID string)
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
