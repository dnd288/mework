// Binary mework-mezon-worker is a standalone worker that uses the Mezon
// turbo SDK to connect multiple Mezon bots concurrently. Messages flow
// through a Redis-backed inbox/outbox queue system:
//
//   Mezon → worker → [INBOX queue] → orchestrator (Claude) → [OUTBOX queue] → worker → Mezon
//   CLI   → worker → [INBOX queue] ↗                                   ↖ CLI reads from OUTBOX
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/mezon/mezon-go-sdk-turbo/lib/rest"
	"github.com/mezon/mezon-go-sdk-turbo/lib/tier"
	mezon_turbo "github.com/mezon/mezon-go-sdk-turbo/lib/turbo"
	"github.com/mezon/mezon-go-sdk-turbo/lib/types"

	"mework/libs/client/runner"
)
// Build-time variables — injected via -ldflags (see .goreleaser.yml).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)


// inboxMessage is what we store in the inbox queue.
type inboxMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	ClanID    string `json:"clan_id"`
	Mode      int32  `json:"mode"`
	IsPublic  bool   `json:"is_public"`
	BotKeyID  string `json:"bot_key_id"`
	BotToken  string `json:"bot_token"`
	Text      string `json:"text"`
}

// outboxMessage is what we store in the outbox queue.
type outboxMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	ClanID    string `json:"clan_id"`
	Mode      int32  `json:"mode"`
	IsPublic  bool   `json:"is_public"`
	BotKeyID  string `json:"bot_key_id"`
	BotToken  string `json:"bot_token"`
	Response  string `json:"response"`
}

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

	// Connect to Redis (real or embedded miniredis).
	rdb, err := connectRedis(ctx, cfg)
	if err != nil {
		log.Fatalf("redis setup: %v", err)
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

	knownBotIDs := make(map[string]string) // appID → actual Mezon userID

	var engine *mezon_turbo.Engine
	var msgMu sync.Mutex
	msgCounter := 0

	engine = mezon_turbo.New(engineCfg, rdb, restClient,
		func(bot types.BotRef, msg types.Message) {
			log.Printf("inbound: bot=%s channel=%s clan=%s sender=%s",
				bot.KeyID, msg.ChannelID, msg.ClanID, msg.SenderID)

			// Skip echo: messages from the bot's own user ID are echoes.
			if botUserID, ok := knownBotIDs[bot.KeyID]; ok && msg.SenderID == botUserID {
				log.Printf("inbound: skipped echo from bot user %s", botUserID)
				return
			}

			// Only respond if the bot is @mentioned, or the message is
			// a reply to the bot's message (Mezon reply feature).
			mentioned := isMentioned(msg.Mentions, bot.KeyID)
			repliedTo := isReplyTo(msg.References, bot.KeyID)
			if !mentioned && !repliedTo {
				log.Printf("inbound: skipped (not mentioned, not a reply)")
				return
			}

			// Extract text.
			text := extractText(msg.Content)
			if text == "" {
				text = msg.Content
			}

			msgMu.Lock()
			msgCounter++
			msgID := fmt.Sprintf("%s:%d", bot.KeyID, msgCounter)
			msgMu.Unlock()

			inbox := inboxMessage{
				ID:        msgID,
				ChannelID: msg.ChannelID,
				ClanID:    msg.ClanID,
				Mode:      msg.Mode,
				IsPublic:  msg.IsPublic,
				BotKeyID:  bot.KeyID,
				BotToken:  bot.BotToken,
				Text:      text,
			}
			data, _ := json.Marshal(inbox)
			if err := rdb.LPush(ctx, "orchestrator:inbox", data).Err(); err != nil {
				log.Printf("inbound: inbox push error: %v", err)
			} else {
				log.Printf("inbound: pushed to inbox (id=%s, text=%s)", msgID, text)
			}

			// Send typing indicator via WebSocket so Mezon shows
			// "BotName is typing..." in the channel.
			go func() {
				botUserID := knownBotIDs[bot.KeyID]
				if botUserID == "" {
					botUserID = bot.KeyID
				}
				log.Printf("typing: sending indicator (user=%s, channel=%s)", botUserID, msg.ChannelID)
				typing := types.Message{
					ChannelID: msg.ChannelID,
					ClanID:    msg.ClanID,
					Mode:      msg.Mode,
					IsPublic:  msg.IsPublic,
				}
				typingBot := types.BotRef{
					KeyID:     bot.KeyID,
					BotUserID: botUserID,
				}
				// Send typing indicator pulses while Claude processes.
				// Each pulse keeps "bot is typing..." visible for ~5s.
				for i := 0; i < 20; i++ { // up to ~60s of typing
					select {
					case <-ctx.Done():
						return
					default:
						engine.SendTyping(typingBot, typing)
						time.Sleep(3 * time.Second)
					}
				}
			}()

			if cfg.MeworkToken == "" {
				// Local mode: the orchestrator goroutine (started below)
				// will process the inbox and push results to the outbox.
			}
		},
	)

	// Register all configured bots.
	for _, bot := range cfg.Bots {
		session, authErr := restClient.Authenticate(ctx, bot.AppID, bot.APIKey)
		if authErr == nil {
			knownBotIDs[bot.AppID] = session.UserID
		}
		ref := types.BotRef{
			KeyID:     bot.AppID,
			TenantID:  "default",
			BotUserID: bot.AppID,
			BotToken:  bot.APIKey,
			Plan:      bot.Plan,
		}
		engine.Register(ref)
		log.Printf("registered bot %s", bot.AppID)
	}

	// ── Orchestrator dispatcher ──────────────────────────────────────
	// Polls the inbox, runs Claude, pushes results to the outbox.
	// Runs in local mode (no MEWORK_TOKEN). In server mode, the claim-worker
	// handles this via the job queue instead.
	go func() {
		if cfg.MeworkToken != "" {
			// Server mode: skip — the claim-worker polls the job queue.
			return
		}
		backend := env("BACKEND", "claude")
		log.Printf("orchestrator: started (backend=%s)", backend)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				data, err := rdb.RPop(ctx, "orchestrator:inbox").Result()
				if err == redis.Nil {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				if err != nil {
					log.Printf("orchestrator: inbox pop error: %v", err)
					time.Sleep(time.Second)
					continue
				}

				var inbox inboxMessage
				if err := json.Unmarshal([]byte(data), &inbox); err != nil {
					log.Printf("orchestrator: decode error: %v", err)
					continue
				}

				log.Printf("orchestrator: processing inbox item %s", inbox.ID)
				// Prepend response format rules for Mezon delivery.
				prompt := fmt.Sprintf("[Reminder: Mezon format - **bold**, `code`, ```blocks```, lists. No links/tables/headings.]\n\nUser message: %s", inbox.Text)
				result := execute(backend, prompt, "/tmp/orchestrator-workspace")

				response := result.Output
				if result.Error != "" {
					log.Printf("orchestrator: exec error for %s: %v", inbox.ID, result.Error)
					response = "Error: " + result.Error
				}

				outbox := outboxMessage{
					ID:        inbox.ID,
					ChannelID: inbox.ChannelID,
					ClanID:    inbox.ClanID,
					Mode:      inbox.Mode,
					IsPublic:  inbox.IsPublic,
					BotKeyID:  inbox.BotKeyID,
					BotToken:  inbox.BotToken,
					Response:  response,
				}
				outData, _ := json.Marshal(outbox)
				if err := rdb.LPush(ctx, "orchestrator:outbox", outData).Err(); err != nil {
					log.Printf("orchestrator: outbox push error: %v", err)
				} else {
					log.Printf("orchestrator: pushed result for %s to outbox (%d chars)", inbox.ID, len(response))
				}
			}
		}
	}()

	// ── Outbound dispatcher ──────────────────────────────────────────
	// Polls the outbox and sends responses back to Mezon channels.
	go func() {
		log.Println("outbound: started")
		for {
			select {
			case <-ctx.Done():
				return
			default:
				data, err := rdb.RPop(ctx, "orchestrator:outbox").Result()
				if err == redis.Nil {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				if err != nil {
					log.Printf("outbound: outbox pop error: %v", err)
					time.Sleep(time.Second)
					continue
				}

				var outbox outboxMessage
				if err := json.Unmarshal([]byte(data), &outbox); err != nil {
					log.Printf("outbound: decode error: %v", err)
					continue
				}

				log.Printf("outbound: sending response for %s to channel %s (%d chars)",
					outbox.ID, outbox.ChannelID, len(outbox.Response))

				// Post-process: strip unsupported markdown for Mezon.
				cleaned := cleanMezonMarkdown(outbox.Response)

				botRef := types.BotRef{
					KeyID:     outbox.BotKeyID,
					TenantID:  "default",
					BotUserID: outbox.BotKeyID,
					BotToken:  outbox.BotToken,
				}
				reply := types.Message{
					ChannelID: outbox.ChannelID,
					ClanID:    outbox.ClanID,
					Mode:      outbox.Mode,
					IsPublic:  outbox.IsPublic,
				}
				if err := engine.Send(botRef, reply, cleaned, false); err != nil {
					log.Printf("outbound: send error: %v", err)
				} else {
					log.Printf("outbound: reply sent for %s", outbox.ID)
				}
			}
		}
	}()

	// Initialize workspace with orchestrator config + response format rules.
	initOrchestratorWorkspace("/tmp/orchestrator-workspace")

	// Register agent and start socket for "mework agent send <name>".
	// Read agent name from workspace mework.yml, or use env, or fall back to bot ID.
	agentName := os.Getenv("MEWORK_AGENT_NAME")
	if agentName == "" {
		agentName = readWorkspaceName("/tmp/orchestrator-workspace")
	}
	for _, bot := range cfg.Bots {
		name := agentName
		if name == "" {
			name = bot.AppID
		}
		sockPath, _ := runner.SocketPath("/tmp/orchestrator-workspace")
		runner.RegisterOfflineAgent(runner.OfflineAgentInfo{
			Name: name, SocketPath: sockPath, Status: "online",
			Workspace: "/tmp/orchestrator-workspace", Backend: "claude",
		})
		go func(sp string) {
			os.Remove(sp)
			l, e := net.Listen("unix", sp)
			if e != nil { log.Printf("agent socket: %v", e); return }
			os.Chmod(sp, 0600)
			log.Printf("agent socket: %s", sp)
			defer l.Close()
			for {
				c, e := l.Accept()
				if e != nil { if ctx.Err() != nil { return }; continue }
				go func(conn net.Conn) {
					defer conn.Close()
					var req struct {
						Method string          `json:"method"`
						Params json.RawMessage `json:"params"`
						ID     interface{}      `json:"id"`
					}
					if json.NewDecoder(conn).Decode(&req) != nil { return }
					if req.Method != "run" { return }
					var p struct{ Instruction string `json:"instruction"` }
					json.Unmarshal(req.Params, &p)
					if p.Instruction == "" { return }
					msgMu.Lock()
					msgCounter++
					mid := fmt.Sprintf("cli:%d", msgCounter)
					msgMu.Unlock()
					data, _ := json.Marshal(inboxMessage{ID: mid, Text: p.Instruction})
					rdb.LPush(ctx, "orchestrator:inbox", data)
					log.Printf("agent socket: pushed: %s", p.Instruction)
					json.NewEncoder(conn).Encode(map[string]interface{}{"result": map[string]interface{}{"output": "accepted, processing...", "exitCode": 0}, "id": req.ID})
				}(c)
			}
		}(sockPath)
	}

	log.Printf("worker started with %d bots", len(cfg.Bots))

	// Run the turbo engine (blocks until ctx cancelled).
	engine.Run(ctx)
	log.Println("worker stopped")
}

// execute runs a backend command (e.g. Claude) with the given instruction.
type execResult struct {
	Output   string
	ExitCode int
	Error    string
}

func execute(backend, instruction, workDir string) execResult {
	path, err := exec.LookPath(backend)
	if err != nil {
		return execResult{Error: fmt.Sprintf("backend %q not found: %v", backend, err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, path)
	cmd.Dir = workDir // so Claude discovers .claude/settings.json with MCP tools
	cmd.Env = append(os.Environ(),
		"MEWORK_WORKSPACE_PATH="+workDir,
	)
	cmd.Stdin = strings.NewReader(instruction)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	result := execResult{Output: output, ExitCode: 0}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		result.Error = err.Error()
	}
	return result
}

func connectRedis(ctx context.Context, cfg *Config) (redis.Cmdable, error) {
	if cfg.RedisURL != "" {
		redisOpts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
		}
		rdb := redis.NewClient(redisOpts)
		if err := rdb.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		log.Println("redis connected")
		return rdb, nil
	}

	s, err := miniredis.Run()
	if err != nil {
		return nil, fmt.Errorf("miniredis start: %w", err)
	}
	log.Println("WARNING: using embedded in-memory Redis (state lost on restart)")
	log.Println("  Set REDIS_URL=redis://... for persistent state")

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("miniredis ping: %w", err)
	}
	return rdb, nil
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


// initOrchestratorWorkspace initializes orchestrator workspace from .mework/<role>/ template.
func initOrchestratorWorkspace(dir string) {
	if dir == "" {
		return
	}
	if err := initFromTemplate(dir, "orchestrator"); err != nil {
		log.Printf("workspace: template init failed (%v), using default", err)
		initDefaultWorkspace(dir)
	} else {
		log.Printf("workspace initialized: %s", dir)
	}
}



// initFromTemplate recursively copies .mework/<role>/ template to dir.
func initFromTemplate(dir, role string) error {
	tmplRoot := findTemplateDir(role)
	if tmplRoot == "" {
		return fmt.Errorf("template .mework/%s not found", role)
	}
	mcpBin := resolveMCPBin()
	return copyTemplateTree(tmplRoot, dir, mcpBin)
}

// copyTemplateTree recursively copies a directory tree, replacing __MCP_BIN_PATH__.
func copyTemplateTree(src, dst, mcpBin string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := src + "/" + entry.Name()
		dstPath := dst + "/" + entry.Name()
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0700); err != nil {
				return err
			}
			if err := copyTemplateTree(srcPath, dstPath, mcpBin); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			content := strings.ReplaceAll(string(data), "__MCP_BIN_PATH__", mcpBin)
			if err := os.WriteFile(dstPath, []byte(content), 0600); err != nil {
				return err
			}
		}
	}
	return nil
}

// findTemplateDir locates .mework/<role>/ relative to executable or project root.
func findTemplateDir(role string) string {
	rel := ".mework/" + role
	canonical := "templates/workspace/" + role
	candidates := []string{canonical, rel, "examples/remote-claude/" + rel}
	if exe, err := os.Executable(); err == nil {
		if idx := strings.LastIndex(exe, "/"); idx >= 0 {
			base := exe[:idx]
			candidates = append(candidates,
				base+"/"+canonical,
				base+"/../"+canonical,
				base+"/../../"+canonical,
				base+"/"+rel,
				base+"/../"+rel,
				base+"/../../examples/remote-claude/"+rel,
			)
		}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return ""
}

// resolveMCPBin finds mework-mcp adjacent to worker binary or in GOPATH.
func resolveMCPBin() string {
	if exe, err := os.Executable(); err == nil {
		if idx := strings.LastIndex(exe, "/"); idx >= 0 {
			base := exe[:idx]
			for _, c := range []string{
				base + "/mework-mcp",
				base + "/../bin/mework-mcp",
				base + "/../../bin/mework-mcp",
			} {
				if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
					return c
				}
			}
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		if fi, err := os.Stat(home + "/go/bin/mework-mcp"); err == nil && !fi.IsDir() {
			return home + "/go/bin/mework-mcp"
		}
	}
	return "mework-mcp"
}

// initDefaultWorkspace creates a minimal workspace (fallback when no template).
func initDefaultWorkspace(dir string) {
	_ = os.MkdirAll(dir, 0700)
	yml := "name: orchestrator\nversion: \"1.0.0\"\nengine: local\nbackend: claude\nrole: orchestrator\n"
	_ = os.WriteFile(dir+"/mework.yml", []byte(yml), 0600)
	md := "# Orchestrator\nRespond in Mezon-compatible format.\n"
	_ = os.WriteFile(dir+"/CLAUDE.md", []byte(md), 0600)
}




func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// isReplyTo checks if a references JSON blob contains a reference to the
// given user ID (i.e., the message is a reply to that user's message).
// Format: [{"message_sender_id": "...", ...}, ...]
func isReplyTo(refsJSON, userID string) bool {
	if refsJSON == "" || refsJSON == "null" || refsJSON == "[]" {
		return false
	}
	var refs []struct {
		SenderID string `json:"message_sender_id"`
	}
	if err := json.Unmarshal([]byte(refsJSON), &refs); err != nil {
		return false
	}
	for _, r := range refs {
		if r.SenderID == userID {
			return true
		}
	}
	return false
}

// isMentioned checks if a mentions JSON blob contains a mention of the
// given user ID. The blob format is typically [{"user_id": "..."}, ...].
func isMentioned(mentionsJSON, userID string) bool {
	if mentionsJSON == "" || mentionsJSON == "null" || mentionsJSON == "[]" {
		return false
	}
	var mentions []struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(mentionsJSON), &mentions); err != nil {
		return false
	}
	for _, m := range mentions {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

// cleanMezonMarkdown strips markdown features that Mezon doesn't support.
// Keeps: **bold**, `code`, ```blocks```, - lists, paragraphs
// Removes: [links](url), images, headings, tables, blockquotes, HRs
func cleanMezonMarkdown(s string) string {
	// Remove image syntax: ![alt](url)
	s = imgRe.ReplaceAllString(s, "")
	// Remove link syntax but keep the text: [text](url) → text
	s = linkRe.ReplaceAllString(s, "$1")
	// Remove headings: ## text → text
	s = headingRe.ReplaceAllString(s, "$1")
	// Remove blockquotes: > text → text
	s = blockquoteRe.ReplaceAllString(s, "$1")
	// Remove horizontal rules: ---, ***, ___
	s = hrRe.ReplaceAllString(s, "")
	// Remove tables: | col | col | lines
	s = tableRowRe.ReplaceAllString(s, "")
	s = tableSepRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

var (
	imgRe        = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	linkRe       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	headingRe    = regexp.MustCompile(`(?m)^#{1,6}\s+(.*)$`)
	blockquoteRe = regexp.MustCompile(`(?m)^>\s?(.*)$`)
	hrRe         = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	tableRowRe   = regexp.MustCompile(`(?m)^\|.*\|$`)
	tableSepRe   = regexp.MustCompile(`(?m)^\|[-:| ]+\|$`)
)

// readWorkspaceName reads the `name:` field from a mework.yml file.
func readWorkspaceName(dir string) string {
	data, err := os.ReadFile(dir + "/mework.yml")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}

func extractText(content string) string {
	if len(content) < 3 || content[0] != '{' {
		return ""
	}
	var parsed struct {
		T string `json:"t"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	return parsed.T
}
