package runner

import (
	"fmt"
	"strings"

	"mework/libs/client/subscribe"
	"mework/libs/shared/core"
)

func buildPrompt(job *subscribe.Job) string {
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

func formatResult(backend string, res core.Result) string {
	if res.Error != "" {
		return fmt.Sprintf("⚠️ Agent (%s) failed (exit %d): %s\n\n```\n%s\n```",
			backend, res.ExitCode, res.Error, truncate(res.Output, 4000))
	}
	return fmt.Sprintf("✅ Agent (%s) finished:\n\n```\n%s\n```", backend, truncate(res.Output, 8000))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}
