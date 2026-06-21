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
	sessionsvc "mework/server/session"
	"mework/server/webhook"
	"mework/shared/grant"
)

type Server struct {
	Router *chi.Mux
	Pool   *pgxpool.Pool
	Config *Config
}

func NewServer(pool *pgxpool.Pool, cfg *Config) *Server {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	r.Get("/healthz", HealthHandler(pool))

	patAuth := auth.NewPATAuthenticator(pool, cfg.MelloBaseURL)
	registrySvc := registry.NewService(pool, cfg.ServerKey)
	registryHandlers := registry.NewHandlers(registrySvc)

	connectionSvc := connection.NewService(pool, cfg.MeworkSecretKey)
	connectionHandlers := connection.NewHandlers(connectionSvc)

	profileSvc := catalog.NewService(pool)
	profileHandlers := catalog.NewHandlers(profileSvc)

	msgBroker := cfg.Broker
	if msgBroker == nil {
		msgBroker = memory.New()
	}

	agentHandlers := catalog.NewAgentHandlersWithSelector(profileSvc, msgBroker, orchestrator.NewRunnerSelector())
	sseHandler := bus.NewSSEHandler(msgBroker)
	msgAckHandler := bus.NewAckHandler(msgBroker)

	sessionManager := sessionsvc.NewManager(msgBroker, sessionsvc.DefaultConfig())
	sessionHandlers := sessionsvc.NewHandlers(sessionManager)

	webhookHandler := webhook.NewHandler(pool, msgBroker, cfg.MeworkSecretKey, cfg.MelloBaseURL)

	melloAdapter := melloprovider.NewMelloAdapter(cfg.MelloBaseURL)
	provider.Register(melloAdapter)

	r.Post("/webhooks/{provider}", webhookHandler.ServeHTTP)

	runtimeAuth := middleware.NewRuntimeAuthenticator(pool, cfg.ServerKey)
	ackHandlers := orchestrator.NewAckHandlers(pool, cfg.MeworkSecretKey, cfg.MelloBaseURL)

	r.Route("/api/v1/jobs", func(r chi.Router) {
		r.Use(runtimeAuth.Middleware)
		r.Post("/{id}/ack", ackHandlers.AckJob)
		r.Post("/{id}/heartbeat", ackHandlers.Heartbeat)
		r.Get("/subscribe", sseHandler.Subscribe)
		r.Post("/messages/{msgID}/ack", msgAckHandler.Ack)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(patAuth.Middleware)

		r.Post("/runtimes", registryHandlers.CreateRuntime)
		r.Get("/runtimes", registryHandlers.ListRuntimes)
		r.Delete("/runtimes/{id}", registryHandlers.DeleteRuntime)

		r.Post("/connections", connectionHandlers.CreateConnection)
		r.Get("/connections", connectionHandlers.ListConnections)
		r.Get("/connections/{provider_code}", connectionHandlers.GetConnection)
		r.Delete("/connections/{provider_code}", connectionHandlers.DeleteConnection)

		r.Post("/profiles", profileHandlers.CreateProfile)
		r.Get("/profiles", profileHandlers.ListProfiles)
		r.Get("/profiles/{name}", profileHandlers.GetProfile)
		r.Put("/profiles/{name}", profileHandlers.UpdateProfile)
		r.Delete("/profiles/{name}", profileHandlers.DeleteProfile)

		r.Post("/agents/{name}/versions", agentHandlers.PublishVersion)
		r.Get("/agents", agentHandlers.ListAgents)
		r.Get("/agents/{name}", agentHandlers.ResolveAgent)
		r.Post("/agents/{name}/dispatch", agentHandlers.Dispatch)

		r.Post("/sessions", sessionHandlers.CreateSession)
		r.Get("/sessions", sessionHandlers.ListSessions)
		r.Get("/sessions/{id}", sessionHandlers.GetSession)
		r.Post("/sessions/{id}/attach", sessionHandlers.AttachSession)
		r.Delete("/sessions/{id}", sessionHandlers.CloseSession)

		r.Post("/runners/registration-tokens", registryHandlers.IssueRegistrationToken)
	})

	r.Post("/api/v1/runners/enroll", registryHandlers.EnrollRunner)

	r.Route("/api/v1/agents", func(r chi.Router) {
		r.Use(runtimeAuth.Middleware)
		r.With(
			middleware.GrantMiddleware([]byte(cfg.ServerKey)),
			middleware.RequireOperation(grant.OpPullAgent),
		).Get("/{name}/versions/{version}/pull", agentHandlers.PullVersion)
	})

	return &Server{
		Router: r,
		Pool:   pool,
		Config: cfg,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}
