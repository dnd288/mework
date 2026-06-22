package hub

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/auth"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/catalog"
	"mework/libs/server/channel"
	"mework/libs/server/connection"
	"mework/libs/server/middleware"
	"mework/libs/server/notify"
	"mework/libs/server/orchestrator"
	"mework/libs/server/provider"
	melloprovider "mework/libs/server/provider/mello"
	"mework/libs/server/registry"
	"mework/libs/server/session"
	"mework/libs/server/webhook"
	"mework/libs/shared/grant"
)

// maxRequestBytes bounds request body size to mitigate memory-exhaustion DoS.
const maxRequestBytes = 4 << 20 // 4 MiB

type Server struct {
	Router           *chi.Mux
	Pool             *pgxpool.Pool
	Config           *Config
	Notifier         *notify.Notifier
	ArtifactHandlers *ArtifactHandlers
}

func NewServer(pool *pgxpool.Pool, cfg *Config) *Server {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	// Cap request bodies to bound memory use (e.g. webhook payloads read via
	// io.ReadAll). SSE responses are unaffected — this limits the request body,
	// not the long-lived response stream.
	r.Use(chimiddleware.RequestSize(maxRequestBytes))

	r.Get("/healthz", HealthHandler(pool))
	r.Get("/livez", LivenessHandler())
	r.Get("/readyz", ReadinessHandler(pool))

	patAuth := auth.NewPATAuthenticator(pool, cfg.MelloBaseURL)
	registrySvc := registry.NewService(pool, cfg.ServerKey)
	registryHandlers := registry.NewHandlers(registrySvc, nil)

	connectionSvc := connection.NewService(pool, cfg.MeworkSecretKey)
	connectionHandlers := connection.NewHandlers(connectionSvc)

	profileSvc := catalog.NewService(pool)
	profileHandlers := catalog.NewHandlers(profileSvc)

	msgBroker := cfg.Broker
	if msgBroker == nil {
		msgBroker = memory.New()
	}

	// Channel routing infrastructure.
	// Experimental channel routing is opt-in (CHANNEL_ROUTING_ENABLED), off by
	// default — a default deployment uses the legacy webhook pipeline.
	channelFeature := channel.NewFeatureFlag(cfg.ChannelRoutingEnabled)
	channelReg := channel.NewPostgresRegistry(pool)
	if err := channelReg.PopulateCache(context.Background()); err != nil {
		log.Printf("Failed to populate channel cache: %v", err)
	}

	agentHandlers := catalog.NewAgentHandlers(profileSvc, msgBroker, nil, nil)
	sseHandler := bus.NewSSEHandler(msgBroker)
	msgAckHandler := bus.NewAckHandler(msgBroker)

	sessionMgr := session.NewManager(msgBroker, session.DefaultConfig())
	sessionHandlers := session.NewHandlers(sessionMgr, agentHandlers, msgBroker)

	autoProvisioner := channel.NewAutoProvisioner(registrySvc, channelReg, sessionMgr, agentHandlers, msgBroker, registry.DefaultTenantID)
	channelRouter := channel.NewRouter(channelReg, msgBroker, autoProvisioner, channelFeature)

	webhookHandler := webhook.NewHandler(pool, msgBroker, cfg.MeworkSecretKey, cfg.MelloBaseURL, channelRouter)

	melloAdapter := melloprovider.NewMelloAdapter(cfg.MelloBaseURL)
	provider.Register(melloAdapter)

	r.Post("/webhooks/{provider}", webhookHandler.ServeHTTP)

	runtimeAuth := middleware.NewRuntimeAuthenticator(pool, cfg.ServerKey)
	ackHandlers := orchestrator.NewAckHandlers(pool, cfg.MeworkSecretKey, cfg.MelloBaseURL)
	claimHandlers := orchestrator.NewClaimHandlers(pool)

	r.Route("/api/v1/jobs", func(r chi.Router) {
		r.Use(runtimeAuth.Middleware)
		r.Post("/{id}/ack", ackHandlers.AckJob)
		r.Post("/claim", claimHandlers.ClaimJob)
		r.Post("/{id}/heartbeat", ackHandlers.Heartbeat)
		r.Get("/subscribe", sseHandler.Subscribe)
		r.Post("/messages/{msgID}/ack", msgAckHandler.Ack)
	})

	// Notifier for outbound notifications.
	notifierSvc := notify.NewNotifier(pool)

	// Artifact store (currently dummy; real ObjectStore-backed version
	// activated once the object store is wired).
	artifactStore := NewDummyArtifactStore()
	artifactHandlers := NewArtifactHandlers(artifactStore)

	channelHandlers := channel.NewHandlers(pool)

	r.Route("/api/v1", func(r chi.Router) {
		// PAT-authenticated management routes (operator / human caller). Grouped
		// (rather than r.Use on the mount) so the runtime-authed agent pull route
		// below can live in the same /api/v1 mount without a separate
		// /api/v1/agents sub-mount shadowing these PAT-authed agent routes.
		r.Group(func(r chi.Router) {
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

			r.Post("/runners/registration-tokens", registryHandlers.IssueRegistrationToken)

			// Artifact endpoints: list artifacts for a run, download a single artifact.
			r.Get("/runs/{runID}/artifacts", artifactHandlers.ListArtifacts)
			r.Get("/runs/{runID}/artifacts/{name}", artifactHandlers.GetArtifact)

			// Channel sessions endpoint: list active channel bindings.
			r.Get("/channels", channelHandlers.ListChannels)

			// Interactive session lifecycle (c0031): create dispatches an
			// open-session message to the named runner; owner/tenant come from
			// the authenticated PAT, never from request args.
			r.Post("/sessions", sessionHandlers.CreateSession)
			r.Get("/sessions", sessionHandlers.ListSessions)
			r.Get("/sessions/{id}", sessionHandlers.GetSession)
			r.Delete("/sessions/{id}", sessionHandlers.CloseSession)

			// Session chat bus (c0032, PAT/human): submit a turn and stream events.
			r.Post("/sessions/{id}/messages", sessionHandlers.SendMessage)
			r.Get("/sessions/{id}/stream", sessionHandlers.StreamSession)
		})

		// Runtime-authenticated agent pull (rt_ Bearer + grant). Same /api/v1
		// mount, separate middleware group — coexists with the PAT-authed agent
		// management routes above (distinct paths) with no shadowing.
		r.Group(func(r chi.Router) {
			r.Use(runtimeAuth.Middleware)
			r.With(
				middleware.GrantMiddleware([]byte(cfg.ServerKey)),
				middleware.RequireOperation(grant.OpPullAgent),
			).Get("/agents/{name}/versions/{version}/pull", agentHandlers.PullVersion)
		})
	})

	r.Post("/api/v1/runners/enroll", registryHandlers.EnrollRunner)

	// Runner session endpoints (runtime-auth, rt_ Bearer): the daemon POSTs a
	// terminal result here (c0031) and republishes outgoing ChatEvents (c0032)
	// onto the session control topic for the hub to relay.
	r.Route("/api/v1/runners/sessions", func(r chi.Router) {
		r.Use(runtimeAuth.Middleware)
		r.Post("/{id}/result", sessionHandlers.ResultSession)
		r.Post("/{id}/events", sessionHandlers.ReceiveEvents)
	})

	// Start background notification retry sweeper.
	startNotifySweeper(context.Background(), notifierSvc)

	// Start background channel session sweeper (30s interval).
	channelSweeper := channel.NewSweeper(pool, channelReg, 30*time.Second)
	channelSweeper.Start(context.Background())

	return &Server{
		Router:           r,
		Pool:             pool,
		Config:           cfg,
		Notifier:         notifierSvc,
		ArtifactHandlers: artifactHandlers,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}

// startNotifySweeper launches a background goroutine that retries pending
// notification deliveries every 30 seconds. It stops when the context is
// cancelled.
func startNotifySweeper(ctx context.Context, notifier *notify.Notifier) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := notifier.RetryPending(ctx); err != nil {
					log.Printf("Notification retry sweeper: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
