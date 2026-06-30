// Binary mework-mezon-worker is a standalone worker that uses the Mezon
// turbo SDK to connect multiple Mezon bots concurrently, enqueue received
// messages as jobs via the mework-server API, and poll for completed jobs
// to send replies back to Mezon channels.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mezon/mezon-go-sdk-turbo/lib/rest"
	"github.com/mezon/mezon-go-sdk-turbo/lib/tier"
	mezon_turbo "github.com/mezon/mezon-go-sdk-turbo/lib/turbo"
	"github.com/mezon/mezon-go-sdk-turbo/lib/types"
)

func main() {
	cfg, err := Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down", sig)
		cancel()
	}()

	// Connect to Redis.
	var rdb redis.Cmdable
	if cfg.RedisURL != "" {
		redisOpts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			log.Fatalf("invalid REDIS_URL: %v", err)
		}
		rdb = redis.NewClient(redisOpts)
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Fatalf("redis ping failed: %v", err)
		}
		log.Println("redis connected")
	} else {
		log.Fatal("REDIS_URL is required")
	}

	restClient := rest.New("https://gw.mezon.ai")

	engineCfg := mezon_turbo.Config{
		WSHost: env("MEZON_WS_HOST", "gw.mezon.ai"),
		WSSSL:  true,
		Tier: tier.Config{
			MaxHot:       intEnv("MEZON_MAX_HOT", 200),
			HotPerTenant: intEnv("MEZON_HOT_PER_TENANT", 10),
			WarmPoll:     20 * time.Second,
			ColdPoll:     3 * time.Minute,
			HotIdle:      2 * time.Minute,
			WarmIdle:     10 * time.Minute,
			Tick:         2 * time.Second,
			PlanWeight:   map[string]float64{"starter": 1, "pro": 2, "enterprise": 3},
		},
		PollRPS:       intEnv("MEZON_POLL_RPS", 50),
		PollWorkers:   intEnv("MEZON_POLL_WORKERS", 8),
		PollPageLimit: int32(intEnv("MEZON_POLL_PAGE_LIMIT", 20)),
		StateTTL:      30 * 24 * time.Hour,
		DedupCap:      2048,
		PingInterval:  30 * time.Second,
	}

	// Build bot registry: keyID → BotConfig for reply routing.
	botRegistry := make(map[string]BotConfig)
	for _, b := range cfg.Bots {
		botRegistry[b.AppID] = b
	}

	// Outbound poller: polls server for done jobs → sends replies via engine.
	poller := NewOutboundPoller(cfg)

	// Channel→bot mapping: populated by inbound handler, used by outbound.
	var channelBotMu sync.RWMutex
	channelBotMap := make(map[string]string) // channelID → botKeyID

	// Create the turbo engine.
	engine := mezon_turbo.New(engineCfg, rdb, restClient,
		func(bot types.BotRef, msg types.Message) {
			log.Printf("inbound: bot=%s channel=%s sender=%s msg=%s",
				bot.KeyID, msg.ChannelID, msg.SenderID, truncate(msg.Content, 80))

			// Record channel→bot mapping for reply routing.
			channelBotMu.Lock()
			channelBotMap[msg.ChannelID] = bot.KeyID
			channelBotMu.Unlock()

			// Enqueue the message as a server job.
			messageID := msg.MessageID
			if messageID == "" {
				messageID = msg.ChannelID + ":" + msg.SenderID + ":" + time.Now().String()
			}
			poller.EnqueueJob(ctx, msg.ChannelID, msg.SenderID, msg.Content, messageID)
		},
	)

	// Register all configured bots with the turbo engine.
	// The engine uses the Authenticator interface to exchange API keys
	// for session tokens automatically before dialing, so BotToken
	// should be the raw API key, not a session JWT.
	for _, bot := range cfg.Bots {
		ref := types.BotRef{
			KeyID:     bot.AppID,
			TenantID:  "default",
			BotUserID: bot.AppID,
			BotToken:  bot.APIKey,
			Plan:      bot.Plan,
		}

		// The engine's authenticator (restClient) will exchange the API key
		// for a session token when it needs to dial a WebSocket or make
		// authenticated REST calls. This happens automatically via the
		// Authenticator interface that rest.Client satisfies.
		engine.Register(ref)
		log.Printf("registered bot %s", bot.AppID)
	}

	log.Printf("worker started with %d bots", len(cfg.Bots))

	// Start the outbound poller (polls server → sends replies via engine).
	go func() {
		ticker := time.NewTicker(cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poller.pollAndProcess(ctx, func(channelID, text string) error {
					channelBotMu.RLock()
					botKeyID, ok := channelBotMap[channelID]
					channelBotMu.RUnlock()
					if !ok {
						log.Printf("outbound: no bot known for channel %s", channelID)
						return nil
					}
					botCfg, ok := botRegistry[botKeyID]
					if !ok {
						log.Printf("outbound: no config for bot %s", botKeyID)
						return nil
					}
					botRef := types.BotRef{
						KeyID:     botKeyID,
						TenantID:  "default",
						BotUserID: botCfg.AppID,
						BotToken:  botCfg.APIKey,
					}
					in := types.Message{ChannelID: channelID}
					return engine.Send(botRef, in, text, false)
				})
			}
		}
	}()

	// Run the turbo engine (blocks until ctx cancelled).
	engine.Run(ctx)
	log.Println("worker stopped")
}

func intEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
