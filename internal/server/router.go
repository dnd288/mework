package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
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
