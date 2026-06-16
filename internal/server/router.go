package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/internal/server/auth"
	"mework/internal/server/connection"
	"mework/internal/server/jobs"
	servermiddleware "mework/internal/server/middleware"
	"mework/internal/server/profile"
	"mework/internal/server/provider"
	melloprovider "mework/internal/server/provider/mello"
	"mework/internal/server/registry"
	"mework/internal/server/webhook"
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
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Mount health check
	r.Get("/healthz", HealthHandler(pool))

	// Instantiate services and handlers
	patAuth := auth.NewPATAuthenticator(pool, cfg.MelloBaseURL)
	registrySvc := registry.NewService(pool, cfg.ServerKey)
	registryHandlers := registry.NewHandlers(registrySvc)

	connectionSvc := connection.NewService(pool, cfg.MeworkSecretKey)
	connectionHandlers := connection.NewHandlers(connectionSvc)

	profileSvc := profile.NewService(pool)
	profileHandlers := profile.NewHandlers(profileSvc)

	// Instantiate webhook handler
	webhookHandler := webhook.NewHandler(pool, cfg.MeworkSecretKey, cfg.MelloBaseURL)

	// Register Mello adapter
	melloAdapter := melloprovider.NewMelloAdapter(cfg.MelloBaseURL)
	provider.Register(melloAdapter)

	// Webhook endpoint (unauthenticated, signature-verified inside handler)
	r.Post("/webhooks/{provider}", webhookHandler.ServeHTTP)

	// Instantiate runtime authenticator for daemon endpoints
	runtimeAuth := servermiddleware.NewRuntimeAuthenticator(pool, cfg.ServerKey)

	// Instantiate claim & ack handlers
	claimHandlers := jobs.NewClaimHandlers(pool)
	ackHandlers := jobs.NewAckHandlers(pool, cfg.MeworkSecretKey, cfg.MelloBaseURL)

	// API routes group under runtime (rt_token) authentication
	r.Route("/api/v1/jobs", func(r chi.Router) {
		r.Use(runtimeAuth.Middleware)

		r.Post("/claim", claimHandlers.ClaimJob)
		r.Post("/{id}/ack", ackHandlers.AckJob)
		r.Post("/{id}/heartbeat", ackHandlers.Heartbeat)
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
