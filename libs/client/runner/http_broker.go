package runner

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"mework/libs/server/bus"
)

// httpBroker is a minimal bus.Broker adapter used on the daemon to egress an
// interactive session's per-turn ChatEvents to the server. Only Publish is
// implemented: it POSTs the marshaled event payload to the server's session
// events-ingress endpoint (runtime-authed) so the per-turn token/message/done
// granularity reaches the CLI via the server relay. Subscribe/Ack are not used
// on the daemon and return errors if called.
type httpBroker struct {
	hubURL string
	secret string
	client *http.Client
}

// newHTTPBroker builds an httpBroker posting to hubURL with the runtime secret.
func newHTTPBroker(hubURL, secret string) *httpBroker {
	return &httpBroker{
		hubURL: strings.TrimRight(hubURL, "/"),
		secret: secret,
		client: http.DefaultClient,
	}
}

// compile-time assertion that httpBroker satisfies bus.Broker.
var _ bus.Broker = (*httpBroker)(nil)

// Publish POSTs the message payload to
// POST /api/v1/runners/sessions/{id}/events. The session id is recovered from
// the topic (session.<id>.control); a non-2xx response is an error.
func (b *httpBroker) Publish(ctx context.Context, topic bus.Topic, msg bus.Message) error {
	id := sessionIDFromTopic(topic)
	if id == "" {
		return fmt.Errorf("httpBroker: cannot derive session id from topic %q", topic)
	}

	url := fmt.Sprintf("%s/api/v1/runners/sessions/%s/events", b.hubURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(msg.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("events ingress returned status %d", resp.StatusCode)
	}
	return nil
}

// Subscribe is not supported on the daemon-side broker.
func (b *httpBroker) Subscribe(context.Context, bus.Identity, bus.Filter, string) (bus.Subscription, error) {
	return nil, fmt.Errorf("httpBroker does not support Subscribe")
}

// Ack is not supported on the daemon-side broker.
func (b *httpBroker) Ack(context.Context, string) error {
	return fmt.Errorf("httpBroker does not support Ack")
}

// sessionIDFromTopic extracts the <id> from "session.<id>.control" (or any
// "session.<id>.*" topic). Returns "" when the topic is not session-scoped.
func sessionIDFromTopic(topic bus.Topic) string {
	parts := strings.Split(string(topic), ".")
	if len(parts) >= 3 && parts[0] == "session" {
		return parts[1]
	}
	return ""
}
