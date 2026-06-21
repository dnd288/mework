package channel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"mework/libs/server/bus"
)

// spyBroker records Publish calls for test assertions in channel router tests.
type spyBroker struct {
	mu        sync.Mutex
	published []publishRecord
}

type publishRecord struct {
	topic bus.Topic
	msg   bus.Message
}

func (s *spyBroker) Publish(_ context.Context, topic bus.Topic, msg bus.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published = append(s.published, publishRecord{topic, msg})
	return nil
}

func (s *spyBroker) Subscribe(_ context.Context, _ bus.Identity, _ bus.Filter, _ string) (bus.Subscription, error) {
	return nil, nil
}

func (s *spyBroker) Ack(_ context.Context, _ string) error {
	return nil
}

func (s *spyBroker) publishedTopics() []bus.Topic {
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := make([]bus.Topic, len(s.published))
	for i, p := range s.published {
		topics[i] = p.topic
	}
	return topics
}

func (s *spyBroker) publishedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.published)
}

// mockRegistry controls Lookup results for router tests.
type mockRegistry struct {
	mu            sync.Mutex
	lookupResults map[string]lookupResult
	bindCalls     []bindCall
	statusMap     map[string]string
}

type lookupResult struct {
	sessionID string
	err       error
}

type bindCall struct {
	channelKey string
	sessionID  string
	runnerID   string
	provider   string
	resourceID string
	spec       string
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		lookupResults: make(map[string]lookupResult),
		statusMap:     make(map[string]string),
	}
}

func (r *mockRegistry) setLookup(channelKey, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookupResults[channelKey] = lookupResult{sessionID: sessionID}
}

func (r *mockRegistry) setLookupError(channelKey string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookupResults[channelKey] = lookupResult{err: err}
}

func (r *mockRegistry) Bind(_ context.Context, channelKey, sessionID, runnerID, provider, resourceID, spec string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindCalls = append(r.bindCalls, bindCall{
		channelKey: channelKey,
		sessionID:  sessionID,
		runnerID:   runnerID,
		provider:   provider,
		resourceID: resourceID,
		spec:       spec,
	})
	return nil
}

func (r *mockRegistry) Unbind(_ context.Context, channelKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lookupResults, channelKey)
	return nil
}

func (r *mockRegistry) Lookup(_ context.Context, channelKey string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, ok := r.lookupResults[channelKey]
	if !ok {
		return "", nil
	}
	if res.err != nil {
		return "", res.err
	}
	return res.sessionID, nil
}

func (r *mockRegistry) RunnerActiveChannelCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (r *mockRegistry) PopulateCache(_ context.Context) error {
	return nil
}

func (r *mockRegistry) Status(_ context.Context, channelKey string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusMap[channelKey], nil
}

func (r *mockRegistry) SetStatus(_ context.Context, channelKey, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusMap[channelKey] = status
	return nil
}

// mockProvisioner records provision calls for router tests.
type mockProvisioner struct {
	mu              sync.Mutex
	provisionCalls  []provisionCall
	provisionResult string
	provisionErr    error
}

type provisionCall struct {
	providerCode string
	resourceID   string
	spec         string
}

func newMockProvisioner() *mockProvisioner {
	return &mockProvisioner{}
}

func (p *mockProvisioner) setResult(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provisionResult = sessionID
}

func (p *mockProvisioner) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provisionErr = err
}

func (p *mockProvisioner) Provision(ctx context.Context, providerCode, resourceID, spec string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provisionCalls = append(p.provisionCalls, provisionCall{
		providerCode: providerCode,
		resourceID:   resourceID,
		spec:         spec,
	})
	return p.provisionResult, p.provisionErr
}

func (p *mockProvisioner) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.provisionCalls)
}

// mockFeatureFlag controls feature flag state for router tests.
type mockFeatureFlag struct {
	enabled bool
}

func newMockFeatureFlag(enabled bool) *mockFeatureFlag {
	return &mockFeatureFlag{enabled: enabled}
}

func (f *mockFeatureFlag) IsEnabled() bool {
	return f.enabled
}

func (f *mockFeatureFlag) SetEnabled(v bool) {
	f.enabled = v
}

func TestRouter_ChannelKey(t *testing.T) {
	tests := []struct {
		name         string
		providerCode string
		resourceID   string
		want         string
	}{
		{
			name:         "mello ticket",
			providerCode: "mello",
			resourceID:   "TICKET-99",
			want:         "mello:TICKET-99",
		},
		{
			name:         "github issue",
			providerCode: "github",
			resourceID:   "42",
			want:         "github:42",
		},
		{
			name:         "jira epic",
			providerCode: "jira",
			resourceID:   "EPIC-101",
			want:         "jira:EPIC-101",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newMockRegistry()
			broker := &spyBroker{}
			prov := newMockProvisioner()
			feature := newMockFeatureFlag(true)

			router := NewRouter(reg, broker, prov, feature)
			got := router.ChannelKey(tt.providerCode, tt.resourceID)
			if got != tt.want {
				t.Errorf("ChannelKey(%q, %q) = %q, want %q", tt.providerCode, tt.resourceID, got, tt.want)
			}
		})
	}
}

func TestRouter_Route_ToExistingSession(t *testing.T) {
	tests := []struct {
		name         string
		providerCode string
		resourceID   string
		eventType    string
		sessionID    string
		wantTopic    bus.Topic
	}{
		{
			name:         "mello ticket event routed to channel topic",
			providerCode: "mello",
			resourceID:   "TICKET-99",
			eventType:    "dispatch",
			sessionID:    "session-123",
			wantTopic:    bus.FormatChannelTopic("mello", "TICKET-99", "dispatch"),
		},
		{
			name:         "github issue event routed to channel topic",
			providerCode: "github",
			resourceID:   "42",
			eventType:    "dispatch",
			sessionID:    "session-456",
			wantTopic:    bus.FormatChannelTopic("github", "42", "dispatch"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newMockRegistry()
			channelKey := tt.providerCode + ":" + tt.resourceID
			reg.setLookup(channelKey, tt.sessionID)

			broker := &spyBroker{}
			prov := newMockProvisioner()
			feature := newMockFeatureFlag(true)

			router := NewRouter(reg, broker, prov, feature)
			err := router.Route(context.Background(), tt.providerCode, tt.resourceID, tt.eventType, []byte(`{"key":"val"}`))
			if err != nil {
				t.Fatalf("Route(%s, %s, %s): unexpected error: %v", tt.providerCode, tt.resourceID, tt.eventType, err)
			}

			topics := broker.publishedTopics()
			if len(topics) != 1 {
				t.Fatalf("expected 1 publish, got %d", len(topics))
			}
			if topics[0] != tt.wantTopic {
				t.Errorf("Route published to topic %q, want %q", topics[0], tt.wantTopic)
			}
		})
	}
}

func TestRouter_Route_NoSessionTriggersAutoProvision(t *testing.T) {
	tests := []struct {
		name         string
		providerCode string
		resourceID   string
		eventType    string
		spec         string
		sessionID    string
	}{
		{
			name:         "no session triggers provision for mello ticket",
			providerCode: "mello",
			resourceID:   "TICKET-99",
			eventType:    "dispatch",
			spec:         "claude-code",
			sessionID:    "new-session-1",
		},
		{
			name:         "no session triggers provision for github issue",
			providerCode: "github",
			resourceID:   "42",
			eventType:    "dispatch",
			spec:         "codex",
			sessionID:    "new-session-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newMockRegistry()
			broker := &spyBroker{}
			prov := newMockProvisioner()
			prov.setResult(tt.sessionID)
			feature := newMockFeatureFlag(true)

			router := NewRouter(reg, broker, prov, feature)
			err := router.Route(context.Background(), tt.providerCode, tt.resourceID, tt.eventType, []byte(`{"key":"val"}`))
			if err != nil {
				t.Fatalf("Route(%s, %s, %s): unexpected error: %v", tt.providerCode, tt.resourceID, tt.eventType, err)
			}

			if prov.callCount() != 1 {
				t.Fatalf("expected 1 provision call, got %d", prov.callCount())
			}

			// After provision, the event should be published
			topics := broker.publishedTopics()
			if len(topics) != 1 {
				t.Fatalf("expected 1 publish after provision, got %d", len(topics))
			}
			expectedTopic := bus.FormatChannelTopic(tt.providerCode, tt.resourceID, tt.eventType)
			if topics[0] != expectedTopic {
				t.Errorf("Route published to topic %q, want %q", topics[0], expectedTopic)
			}
		})
	}
}

func TestRouter_Route_SequentialDelivery(t *testing.T) {
	reg := newMockRegistry()
	channelKey := "mello:TICKET-99"
	reg.setLookup(channelKey, "session-1")

	broker := &spyBroker{}
	prov := newMockProvisioner()
	feature := newMockFeatureFlag(true)

	router := NewRouter(reg, broker, prov, feature)
	ctx := context.Background()

	// Send two publishes for the same channel concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(n int) {
			defer wg.Done()
			err := router.Route(ctx, "mello", "TICKET-99", "dispatch", []byte(`{"seq":`+string(rune('0'+n))+`}`))
			errs <- err
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Route returned error: %v", err)
		}
	}

	// Both events should have been published (2 publishes)
	if broker.publishedCount() != 2 {
		t.Errorf("expected 2 publishes for sequential delivery, got %d", broker.publishedCount())
	}
}

func TestRouter_Route_ConcurrentDeliveryAcrossResources(t *testing.T) {
	reg := newMockRegistry()
	reg.setLookup("mello:TICKET-99", "session-1")
	reg.setLookup("github:42", "session-2")

	broker := &spyBroker{}
	prov := newMockProvisioner()
	feature := newMockFeatureFlag(true)

	router := NewRouter(reg, broker, prov, feature)
	ctx := context.Background()

	// Publish to two different channels concurrently — should not block
	var wg sync.WaitGroup
	wg.Add(2)

	errs := make(chan error, 2)
	go func() {
		defer wg.Done()
		errs <- router.Route(ctx, "mello", "TICKET-99", "dispatch", []byte(`{}`))
	}()
	go func() {
		defer wg.Done()
		errs <- router.Route(ctx, "github", "42", "dispatch", []byte(`{}`))
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(errs)
		close(done)
	}()

	select {
	case <-done:
		// Both completed — concurrent delivery works
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Route calls timed out — likely blocked on serialization across resources")
	}

	for err := range errs {
		if err != nil {
			t.Errorf("Route returned error: %v", err)
		}
	}

	// Both publishes should have landed
	if broker.publishedCount() != 2 {
		t.Errorf("expected 2 publishes across two resources, got %d", broker.publishedCount())
	}
}

func TestRouter_Route_FeatureFlagOff(t *testing.T) {
	tests := []struct {
		name         string
		providerCode string
		resourceID   string
	}{
		{
			name:         "feature flag off returns nil without provisioning",
			providerCode: "mello",
			resourceID:   "TICKET-99",
		},
		{
			name:         "feature flag off with github resource",
			providerCode: "github",
			resourceID:   "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newMockRegistry()
			broker := &spyBroker{}
			prov := newMockProvisioner()
			feature := newMockFeatureFlag(false) // Feature flag OFF

			router := NewRouter(reg, broker, prov, feature)
			err := router.Route(context.Background(), tt.providerCode, tt.resourceID, "dispatch", []byte(`{}`))
			if err != nil {
				t.Fatalf("Route with feature flag off: unexpected error: %v", err)
			}

			if broker.publishedCount() != 0 {
				t.Errorf("expected 0 publishes with feature flag off, got %d", broker.publishedCount())
			}
			if prov.callCount() != 0 {
				t.Errorf("expected 0 provision calls with feature flag off, got %d", prov.callCount())
			}
		})
	}
}

func TestRouter_Route_DrainingChannelRejects(t *testing.T) {
	reg := newMockRegistry()
	broker := &spyBroker{}
	prov := newMockProvisioner()
	feature := newMockFeatureFlag(true)

	router := NewRouter(reg, broker, prov, feature)
	ctx := context.Background()

	// Transition the channel to draining state before routing
	channelKey := router.ChannelKey("mello", "TICKET-99")
	err := TransitionStatus(ctx, reg, channelKey, StatusActive, StatusDraining)
	if err != nil {
		t.Fatalf("TransitionStatus to draining: %v", err)
	}

	err = router.Route(ctx, "mello", "TICKET-99", "dispatch", []byte(`{}`))
	if err != nil {
		t.Fatalf("Route on draining channel: unexpected error: %v", err)
	}

	if broker.publishedCount() != 0 {
		t.Errorf("expected 0 publishes on draining channel, got %d", broker.publishedCount())
	}
	if prov.callCount() != 0 {
		t.Errorf("expected 0 provision calls on draining channel, got %d", prov.callCount())
	}
}

func TestRouter_Route_ProvisionErrorFallsBackToOldPath(t *testing.T) {
	reg := newMockRegistry()
	broker := &spyBroker{}
	prov := newMockProvisioner()
	prov.setError(errors.New("no eligible worker"))
	feature := newMockFeatureFlag(true)

	router := NewRouter(reg, broker, prov, feature)
	err := router.Route(context.Background(), "mello", "TICKET-99", "dispatch", []byte(`{}`))
	// When provision fails, Route should log the error and return nil
	// (caller falls through to the old publish path)
	if err != nil {
		t.Fatalf("Route after provision error: expected nil, got %v", err)
	}

	if prov.callCount() != 1 {
		t.Errorf("expected 1 provision call, got %d", prov.callCount())
	}
	if broker.publishedCount() != 0 {
		t.Errorf("expected 0 publishes when provision fails, got %d", broker.publishedCount())
	}
}
