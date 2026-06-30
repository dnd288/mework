// Package turbo provides the Mezon bot registry store and HTTP handlers
// for the server API. The turbo engine itself runs in the standalone
// mework-mezon-worker process.
package turbo

import "time"

// BotRow is the database model for mezon_bots.
type BotRow struct {
	ID          string
	TenantID    string
	AccountID   string
	Name        string
	AppID       string
	BaseURL     string
	Status      string // active | inactive
	Plan        string // starter | pro | enterprise
	WorkspaceID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RegisterBotRequest is the API request body for registering a Mezon bot.
type RegisterBotRequest struct {
	Name        string `json:"name"`
	AppID       string `json:"app_id"`
	APIKey      string `json:"api_key"` // plaintext; sealed at rest
	BaseURL     string `json:"base_url,omitempty"`
	Plan        string `json:"plan,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// UpdateBotStatusRequest is the request body for activating/deactivating a bot.
type UpdateBotStatusRequest struct {
	Status string `json:"status"` // "active" or "inactive"
}

// BotResponse is the API response for a bot (never includes API key).
type BotResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AppID       string `json:"app_id"`
	BaseURL     string `json:"base_url"`
	Status      string `json:"status"`
	Plan        string `json:"plan"`
	WorkspaceID string `json:"workspace_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toBotResponse(row *BotRow) BotResponse {
	return BotResponse{
		ID:          row.ID,
		Name:        row.Name,
		AppID:       row.AppID,
		BaseURL:     row.BaseURL,
		Status:      row.Status,
		Plan:        row.Plan,
		WorkspaceID: row.WorkspaceID,
		CreatedAt:   row.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   row.UpdatedAt.Format(time.RFC3339),
	}
}
