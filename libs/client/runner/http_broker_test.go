package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"mework/libs/server/bus"
	"mework/libs/server/session"
)

// TestHTTPBroker_PublishPostsEvent asserts that httpBroker.Publish POSTs the
// ChatEvent payload to POST /api/v1/runners/sessions/{id}/events with the runtime
// credential, deriving the session id from the control topic. Realises delta-spec
// scenario "Per-turn events reach the server".
func TestHTTPBroker_PublishPostsEvent(t *testing.T) {
	const secret = "rt-secret"
	const sessionID = "sess-egress"

	var gotPath, gotAuth, gotMethod string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	broker := newHTTPBroker(srv.URL, secret)

	ev := session.ChatEvent{Kind: session.EventToken, Content: "hello"}
	payload, _ := json.Marshal(ev)
	topic := bus.FormatTopic(bus.TopicSessionControl, sessionID)

	if err := broker.Publish(context.Background(), topic, bus.Message{Payload: payload}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	wantPath := "/api/v1/runners/sessions/" + sessionID + "/events"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotAuth != "Bearer "+secret {
		t.Errorf("auth = %q, want bearer with runtime secret", gotAuth)
	}
	var decoded session.ChatEvent
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode posted body: %v", err)
	}
	if decoded.Kind != session.EventToken || decoded.Content != "hello" {
		t.Errorf("posted event = %+v, want %+v", decoded, ev)
	}
}

// TestHTTPBroker_PublishErrorsOnNon2xx asserts a non-2xx ingress response is an
// error, and that a non-session topic is rejected before any request.
func TestHTTPBroker_PublishErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	broker := newHTTPBroker(srv.URL, "s")
	topic := bus.FormatTopic(bus.TopicSessionControl, "sess-1")
	if err := broker.Publish(context.Background(), topic, bus.Message{Payload: []byte("{}")}); err == nil {
		t.Error("expected an error on a 500 ingress response")
	}

	if err := broker.Publish(context.Background(), bus.Topic("not.a.session.topic"), bus.Message{Payload: []byte("{}")}); err == nil {
		t.Error("expected an error for a non-session topic")
	}
}
