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

	"mework/internal/server"
	"mework/internal/store"
)

func main() {
	log.Println("Starting mework server...")

	// 1. Load configuration from environment
	cfg, err := server.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// 2. Run migrations on startup
	log.Println("Running database migrations...")
	if err := store.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("Migration error: %v", err)
	}
	log.Println("Database migrations complete.")

	// 3. Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	dbStore, err := store.NewStore(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("Database connection error: %v", err)
	}
	defer dbStore.Close()
	log.Println("Database connection established.")

	// 4. Initialize server
	srvInstance := server.NewServer(dbStore.Pool, cfg)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srvInstance,
	}

	// 5. Graceful shutdown handler
	shutdownErr := make(chan error, 1)
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutdown signal received, shutting down gracefully...")

		// Allow 15 seconds for active connections to finish
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			shutdownErr <- err
			return
		}
		shutdownErr <- nil
	}()

	// 6. Start HTTP server
	log.Printf("Listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server failed: %v", err)
	}

	// 7. Await graceful shutdown completion
	if err := <-shutdownErr; err != nil {
		log.Fatalf("Graceful shutdown failed: %v", err)
	}
	log.Println("Server stopped cleanly.")
}
