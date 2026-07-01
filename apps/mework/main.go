// Command mework is the CLI and local agent daemon.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mework/libs/client/cli"
	"mework/libs/server/hub"
	"mework/libs/server/platform/store"

	// Blank-import required server drivers so the in-process hub has its
	// message bus, object storage, and provider adapters registered.
	_ "mework/libs/server/bus/postgres"
	_ "mework/libs/server/storage/fs"
)

func main() {
	// Wire the in-process hub behind the libs/client seam. libs/client cannot
	// import libs/server (it would pull pgx/chi/goose into the client library);
	// apps/mework lives in the root module, which may import libs/server.
	cli.SetServerStarter(runHub)
	cli.Execute()
}

// runHub boots the hub in-process: load config from the environment, run
// migrations, open the pool, and serve hub.NewServer with signal-based graceful
// shutdown. listen, when non-empty, overrides the configured/default address.
// Body lifted from apps/mework-server/main.go.
func runHub(ctx context.Context, listen string) error {
	log.Println("Starting mework server (in-process)...")

	// 1. Load configuration from environment (validates required vars).
	cfg, err := hub.LoadConfig()
	if err != nil {
		return err
	}
	if listen != "" {
		cfg.ListenAddr = listen
	}

	// 2. Run migrations on startup.
	log.Println("Running database migrations...")
	if err := store.RunMigrations(cfg.DatabaseURL); err != nil {
		return err
	}
	log.Println("Database migrations complete.")

	// 3. Connect to the database.
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	dbStore, err := store.NewStore(connectCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer dbStore.Close()
	log.Println("Database connection established.")

	// 4. Initialize the HTTP server.
	srvInstance := hub.NewServer(dbStore.Pool, cfg)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srvInstance,
		// Slowloris mitigation; no WriteTimeout (SSE is long-lived), IdleTimeout
		// exceeds the SSE heartbeat. Mirrors apps/mework-server.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 5. Graceful shutdown on SIGINT/SIGTERM (or parent context cancel).
	shutdownErr := make(chan error, 1)
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigChan:
		case <-ctx.Done():
		}

		log.Println("Shutdown signal received, shutting down gracefully...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		shutdownErr <- httpServer.Shutdown(shutdownCtx)
	}()

	// 6. Start the HTTP server.
	log.Printf("Listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// 7. Await graceful shutdown completion.
	if err := <-shutdownErr; err != nil {
		return err
	}
	log.Println("Server stopped cleanly.")
	return nil
}
