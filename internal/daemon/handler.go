package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"mework/internal/agentrun"
	"mework/internal/cli"
	"mework/internal/mcp"
	"mework/internal/mello"
)

// handler orchestrates a single triggered ticket: announce start, run the
// agent, then write the result back and update a checklist.
type handler struct {
	profile string
	backend agentrun.Backend
	mcp     *mcp.Client
	rest    *mello.Client
	state   *State
	timeout time.Duration
}

// handle processes one trigger match for a ticket. State is marked in_progress
// BEFORE the agent runs so a crash will not re-trigger on restart.
func (h *handler) handle(ctx context.Context, ticket mello.Ticket, m triggerMatch) {
	commentID := m.Comment.ID
	now := time.Now().UTC().Format(time.RFC3339)
	if err := h.state.Mark(ticket.ID, commentID, "in_progress", now); err != nil {
		log.Printf("state mark in_progress failed for %s: %v", ticket.TicketCode, err)
		return
	}

	if _, err := h.mcp.CreateComment(ctx, ticket.ID, "🤖 Agent started working on this ticket."); err != nil {
		log.Printf("start comment failed for %s: %v", ticket.TicketCode, err)
	}

	prompt := buildPrompt(ticket, m.Comment)
	workDir := agentrun.WorkDir(cli.ProfileDir(h.profile), ticket.ID)
	res := agentrun.Run(ctx, h.backend, prompt, workDir, h.timeout)

	body := formatResult(h.backend.Name, res)
	if _, err := h.mcp.CreateComment(ctx, ticket.ID, body); err != nil {
		log.Printf("done comment failed for %s: %v", ticket.TicketCode, err)
	}

	status := "done"
	if res.Err != nil {
		status = "failed"
	}
	if err := h.state.Mark(ticket.ID, commentID, status, time.Now().UTC().Format(time.RFC3339)); err != nil {
		log.Printf("state mark %s failed for %s: %v", status, ticket.TicketCode, err)
	}
	log.Printf("ticket %s handled (status=%s)", ticket.TicketCode, status)
}

// buildPrompt composes the agent prompt from ticket content + the trigger comment.
func buildPrompt(t mello.Ticket, trigger mello.Comment) string {
	return fmt.Sprintf(
		"You are working a Mello ticket.\n\nTitle: %s\n\nDescription:\n%s\n\nTrigger comment:\n%s\n",
		t.Title, t.Description, trigger.Body,
	)
}

// formatResult renders the agent output (or error) as a comment body.
func formatResult(backend string, res agentrun.RunResult) string {
	if res.Err != nil {
		return fmt.Sprintf("⚠️ Agent (%s) failed (exit %d): %v\n\n```\n%s\n```",
			backend, res.ExitCode, res.Err, truncate(res.Output, 4000))
	}
	return fmt.Sprintf("✅ Agent (%s) finished:\n\n```\n%s\n```", backend, truncate(res.Output, 8000))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}
