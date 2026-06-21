package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HealthServer is a loopback HTTP server that exposes /health and /shutdown so
// the `mework daemon stop` command can request a graceful shutdown.
type HealthServer struct {
	srv    *http.Server
	cancel context.CancelFunc
}

// StartHealthServer binds the per-profile health port on loopback. Calling
// /shutdown invokes cancel, which should tear down the run loop.
func StartHealthServer(profile string, cancel context.CancelFunc) (*HealthServer, error) {
	port := HealthPort(profile)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind health port %d: %w", port, err)
	}

	hs := &HealthServer{cancel: cancel}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("shutting down"))
		// Cancel after responding so the client sees success.
		go hs.cancel()
	})

	hs.srv = &http.Server{Handler: mux}
	go func() { _ = hs.srv.Serve(ln) }()
	return hs, nil
}

// Close stops the health server.
func (h *HealthServer) Close() error {
	if h.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return h.srv.Shutdown(ctx)
}

// RequestShutdown asks a running daemon (via its health port) to stop. Returns
// true if the daemon accepted the request.
func RequestShutdown(profile string, timeout time.Duration) bool {
	port := HealthPort(profile)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/shutdown", port), "text/plain", nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusAccepted
}
