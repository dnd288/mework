package daemon

import (
	"context"
	"log"
	"time"

	"mework/internal/agentrun"
	"mework/internal/cli"
	"mework/internal/mcp"
	"mework/internal/mello"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultKeyword      = "/run"
)

// Run is the daemon's main loop: poll watched boards for ticket comments
// containing the trigger keyword and dispatch an agent run for each new one.
func Run(ctx context.Context, profile string, cfg *cli.Config) error {
	keyword := cfg.Daemon.TriggerKeyword
	if keyword == "" {
		keyword = defaultKeyword
	}
	interval := defaultPollInterval
	if cfg.Daemon.PollIntervalSeconds > 0 {
		interval = time.Duration(cfg.Daemon.PollIntervalSeconds) * time.Second
	}

	rest := mello.NewClient(cli.ResolveBaseURL(nil, cfg), cli.ResolveToken(cfg), 0, "daemon")

	self, err := rest.GetCurrentUser()
	if err != nil {
		return err
	}
	log.Printf("daemon authenticated as %s (%s)", self.Name, self.ID)

	mcpClient, err := mcp.New(ctx, cfg.MCPURL, cli.ResolveToken(cfg), 0)
	if err != nil {
		return err
	}
	defer mcpClient.Close()

	backend, ok := agentrun.Detect(cfg.Daemon.Backends)
	if !ok {
		log.Printf("warning: no AI CLI detected (looked for %v); triggers will be skipped", agentrun.DefaultBackends)
	} else {
		log.Printf("using AI backend %s (%s)", backend.Name, backend.Path)
	}

	state, err := LoadState(profile)
	if err != nil {
		return err
	}

	h := &handler{profile: profile, backend: backend, mcp: mcpClient, rest: rest, state: state}

	log.Printf("polling every %s for keyword %q", interval, keyword)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("daemon stopping")
			return nil
		case <-ticker.C:
			if !ok {
				continue // no backend; nothing to run
			}
			pollOnce(ctx, rest, cfg, self.ID, keyword, h)
		}
	}
}

// pollOnce scans watched boards once for new trigger comments.
func pollOnce(ctx context.Context, rest *mello.Client, cfg *cli.Config, selfID, keyword string, h *handler) {
	boards, err := resolveWatchedBoards(rest, cfg)
	if err != nil {
		logRateAware("list boards", err)
		return
	}
	for _, boardID := range boards {
		tickets, err := rest.ListBoardTickets(boardID)
		if err != nil {
			logRateAware("list tickets", err)
			continue
		}
		for _, t := range tickets {
			comments, err := rest.ListComments(t.ID)
			if err != nil {
				logRateAware("list comments", err)
				continue
			}
			for _, m := range findTriggers(comments, keyword, selfID) {
				if h.state.Handled(t.ID, m.Comment.ID) {
					continue
				}
				h.handle(ctx, t, m)
			}
		}
	}
}

// resolveWatchedBoards returns the board ids to poll: the configured watch list
// if set, otherwise every board across accessible workspaces.
func resolveWatchedBoards(rest *mello.Client, cfg *cli.Config) ([]string, error) {
	if len(cfg.Daemon.WatchBoardIDs) > 0 {
		return cfg.Daemon.WatchBoardIDs, nil
	}
	workspaces, err := rest.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, ws := range workspaces {
		boards, err := rest.ListWorkspaceBoards(ws.ID)
		if err != nil {
			return nil, err
		}
		for _, b := range boards {
			ids = append(ids, b.ID)
		}
	}
	return ids, nil
}

// logRateAware logs an error, noting when it is a rate-limit so operators can
// tune the poll interval.
func logRateAware(op string, err error) {
	if apiErr, ok := err.(*mello.APIError); ok && apiErr.IsRateLimited() {
		log.Printf("%s: rate limited — consider a longer poll interval", op)
		return
	}
	log.Printf("%s: %v", op, err)
}
