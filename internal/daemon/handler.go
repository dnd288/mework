package daemon

import (
	"fmt"
	"strings"

	"mework/internal/agentrun"
	"mework/internal/meworkclient"
)

func buildPrompt(job *meworkclient.Job) string {
	var sb strings.Builder
	if job.ProfileBodySnapshot != nil && *job.ProfileBodySnapshot != "" {
		sb.WriteString(*job.ProfileBodySnapshot)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Task Title: ")
	sb.WriteString(job.TaskTitle)
	sb.WriteString("\n\nDescription:\n")
	sb.WriteString(job.TaskDescription)
	if job.Workflow != "" {
		sb.WriteString("\n\nWorkflow: ")
		sb.WriteString(job.Workflow)
	}
	sb.WriteString("\n\nInstructions:\n")
	sb.WriteString(job.Instructions)
	sb.WriteString("\n")
	return sb.String()
}

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
