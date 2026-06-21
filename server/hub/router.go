package hub

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/server/auth"
	"mework/server/bus"
	"mework/server/bus/memory"
	"mework/server/catalog"
	"mework/server/connection"
	"mework/server/middleware"
	"mework/server/orchestrator"
	"mework/server/provider"
	melloprovider "mework/server/provider/mello"
	"mework/server/registry"
	"mework/server/webhook"
)

// Server holds the server state, router, and configuration.
type Server struct {
	Router *chi.Mux
	Pool   *pgxpool.Pool
	Config *Config
}

// NewServer initializes the HTTP router and mounts core handlers/middleware.
func NewServer(pool *pgxpool.Pool, cfg *Config) *Server {
	r := chi.NewRouter()

	// Standard middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Mount health check
	r.Get("/healthz", HealthHandler(pool))

	// Instantiate services and handlers
	patAuth := auth.NewPATAuthenticator(pool, cfg.MelloBaseURL)
	registrySvc := registry.NewService(pool, cfg.ServerKey)
	registryHandlers := registry.NewHandlers(registrySvc)

	connectionSvc := connection.NewService(pool, cfg.MeworkSecretKey)
	connectionHandlers := connection.NewHandlers(connectionSvc)

	profileSvc := catalog.NewService(pool)
	profileHandlers := catalog.NewHandlers(profileSvc)

	// Instantiate message bus and SSE/ack handlers.
	// Use cfg.Broker when provided (tests share a broker between
	// the harness and the server); fall back to a default in-memory
	// broker for production or standalone test servers.
	msgBroker := cfg.Broker
	if msgBroker == nil {
		msgBroker = memory.New()
	}
	sseHandler := bus.NewSSEHandler(msgBroker)
	msgAckHandler := bus.NewAckHandler(msgBroker)

	// Instantiate webhook handler
	webhookHandler := webhook.NewHandler(pool, msgBroker, cfg.MeworkSecretKey, cfg.MelloBaseURL)

	// Register Mello adapter
	melloAdapter := melloprovider.NewMelloAdapter(cfg.MelloBaseURL)
	provider.Register(melloAdapter)

	// Webhook endpoint (unauthenticated, signature-verified inside handler)
	r.Post("/webhooks/{provider}", webhookHandler.ServeHTTP)

	// Instantiate runtime authenticator for daemon endpoints
	runtimeAuth := middleware.NewRuntimeAuthenticator(pool, cfg.ServerKey)

	// Instantiate ack & heartbeat handlers
	ackHandlers := orchestrator.NewAckHandlers(pool, cfg.MeworkSecretKey, cfg.MelloBaseURL)

	// API routes group under runtime (rt_token) authentication
	r.Route("/api/v1/jobs", func(r chi.Router) {
		r.Use(runtimeAuth.Middleware)

		r.Post("/{id}/ack", ackHandlers.AckJob)
		r.Post("/{id}/heartbeat", ackHandlers.Heartbeat)

		// Message bus SSE subscription and ack endpoints
		r.Get("/subscribe", sseHandler.Subscribe)
		r.Post("/messages/{msgID}/ack", msgAckHandler.Ack)
	})

	// API routes group under PAT authentication
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(patAuth.Middleware)

		// Runtimes CRUD
		r.Post("/runtimes", registryHandlers.CreateRuntime)
		r.Get("/runtimes", registryHandlers.ListRuntimes)
		r.Delete("/runtimes/{id}", registryHandlers.DeleteRuntime)

		// Connections CRUD
		r.Post("/connections", connectionHandlers.CreateConnection)
		r.Get("/connections", connectionHandlers.ListConnections)
		r.Get("/connections/{provider_code}", connectionHandlers.GetConnection)
		r.Delete("/connections/{provider_code}", connectionHandlers.DeleteConnection)

		// Profiles CRUD
		r.Post("/profiles", profileHandlers.CreateProfile)
		r.Get("/profiles", profileHandlers.ListProfiles)
		r.Get("/profiles/{name}", profileHandlers.GetProfile)
		r.Put("/profiles/{name}", profileHandlers.UpdateProfile)
		r.Delete("/profiles/{name}", profileHandlers.DeleteProfile)
	})

	return &Server{
		Router: r,
		Pool:   pool,
		Config: cfg,
	}
}

// ServeHTTP implements the http.Handler interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}
