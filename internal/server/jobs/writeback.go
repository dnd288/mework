package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/internal/mello"
	"mework/internal/server/connection"
)

// ExecuteWriteBack performs the server-side writeback to the provider.
func ExecuteWriteBack(ctx context.Context, pool *pgxpool.Pool, secretKey, melloBaseURL, jobID string) error {
	// 1. Load job details and runtime code (as profile name)
	var accountID, providerCode, externalTaskID, status, lastError, resultSummary string
	var profileName, workflowName string
	err := pool.QueryRow(ctx, `
		SELECT j.account_id, j.provider_code, j.external_task_id, j.status, COALESCE(j.last_error, ''), COALESCE(j.result_summary, ''), r.code, COALESCE(j.workflow, '')
		FROM jobs j
		JOIN runtimes r ON j.runtime_id = r.id
		WHERE j.id = $1
	`, jobID).Scan(&accountID, &providerCode, &externalTaskID, &status, &lastError, &resultSummary, &profileName, &workflowName)

	if err != nil {
		return fmt.Errorf("failed to query job for writeback: %w", err)
	}

	// 2. Load and decrypt provider token
	connectionSvc := connection.NewService(pool, secretKey)
	token, err := connectionSvc.GetDecryptedToken(ctx, accountID, providerCode)
	if err != nil {
		return fmt.Errorf("failed to get decrypted provider token: %w", err)
	}

	// 3. Format comment body
	commentBody := formatComment(profileName, workflowName, status, resultSummary, lastError)

	// 4. Call provider API (Mello REST API)
	if providerCode == "mello" {
		client := mello.NewClient(melloBaseURL, token, 30*time.Second, "mework-server")
		_, err = client.CreateComment(externalTaskID, commentBody)
		if err != nil {
			return fmt.Errorf("provider REST API call failed: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported provider code: %s", providerCode)
	}

	return nil
}

func formatComment(profile, workflow, status, resultSummary, lastError string) string {
	var header string
	if workflow != "" {
		header = fmt.Sprintf("mework %s %s — %s", profile, workflow, status)
	} else {
		header = fmt.Sprintf("mework %s — %s", profile, status)
	}

	var body string
	if status == "done" {
		if resultSummary != "" {
			body = resultSummary
		} else {
			body = "completed, no output"
		}
	} else {
		if resultSummary != "" {
			body = resultSummary
		} else if lastError != "" {
			body = lastError
		} else {
			body = "failed without error message"
		}
	}

	// Truncate body to a safe limit (60KB = 61440 bytes)
	if len(body) > 61440 {
		body = body[:61440] + "\n... [truncated]"
	}

	return header + "\n" + body
}
