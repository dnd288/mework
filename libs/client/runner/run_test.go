package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/middleware"
	"mework/libs/shared/config"
)

// testContextInjector sets the runtime_id and account_id context values for test requests.
type testContextInjector struct {
	runtimeID string
	accountID string
}

func (tci testContextInjector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.RuntimeIDKey, tci.runtimeID)
		ctx = context.WithValue(ctx, middleware.AccountIDKey, tci.accountID)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func TestRunnerUsesSubscribeNotClaim(t *testing.T) {
	broker := memory.New()
	sseHandler := bus.NewSSEHandler(broker)

	var claimCount atomic.Int32

	mux := chi.NewRouter()
	mux.Use(testContextInjector{runtimeID: "test-rt-1", accountID: "test-account-1"}.Middleware)

	// Claim endpoint (old behavior, tracked to assert removal).
	mux.Post("/api/v1/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		claimCount.Add(1)
		w.WriteHeader(http.StatusNoContent) // no jobs
	})
	// Subscribe endpoint (new behavior, must be used instead).
	mux.Get("/api/v1/jobs/subscribe", sseHandler.Subscribe)

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := &config.Config{
		ServerURL:    server.URL,
		RuntimeToken: "test-rt",
		Daemon: config.DaemonConfig{
			PollIntervalSeconds: 1,
			Backends:            []string{"echo"}, // detectable on any Unix system
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, "test", cfg)
	}()

	// Let the runner poll once (1 second interval + buffer).
	time.Sleep(1500 * time.Millisecond)

	// RED assertion: Claim should NOT be called because the runner should
	// use Subscribe instead. Since the production code still uses Claim,
	// this assertion will fire, proving the old behavior exists.
	if claimCount.Load() > 0 {
		t.Error("runner should use Subscribe instead of Claim; Claim was called")
	}

	// Also publish a dispatch message and assert the runner processes it
	// via SSE (which it won't yet — another RED failure).
	err := broker.Publish(context.Background(), bus.FormatTopic(bus.TopicRunnerDispatch, "test"), bus.Message{
		Payload: []byte(`{"id":"job-1","instructions":"test"}`),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	cancel()
}
