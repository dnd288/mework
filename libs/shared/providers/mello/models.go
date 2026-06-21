// Package mello holds typed entities and a REST client mirroring the Mello
// Public API (see ~/src/mello-python-sdk/mello). Field tags match the JSON
// keys the API returns (derived from the SDK's from_dict mappings).
package mello

import "time"

// User is the account that owns an API token or authored an entity.
type User struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	AvatarURL string     `json:"avatar_url,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// Workspace is the top-level container for boards.
type Workspace struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	OwnerID   string     `json:"owner_id"`
	Role      string     `json:"role"`
	ImageURL  string     `json:"image_url,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// WorkspaceMember is a user's membership record in a workspace.
type WorkspaceMember struct {
	WorkspaceID string     `json:"workspace_id"`
	UserID      string     `json:"user_id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

// Label is a colored tag attached to tickets, scoped to a board.
type Label struct {
	ID      string `json:"id"`
	BoardID string `json:"board_id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

// TicketMember is a user assigned to (watching) a ticket.
type TicketMember struct {
	TicketID  string     `json:"ticket_id"`
	UserID    string     `json:"user_id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	AvatarURL string     `json:"avatar_url,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// Board is a kanban board holding ordered columns.
type Board struct {
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspace_id"`
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	BackgroundColor string     `json:"background_color,omitempty"`
	CoverImageURL   string     `json:"cover_image_url,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	Columns         []Column   `json:"columns,omitempty"`
}

// Column is an ordered lane on a board containing tickets.
type Column struct {
	ID          string     `json:"id"`
	BoardID     string     `json:"board_id"`
	Name        string     `json:"name"`
	Position    int        `json:"position"`
	TicketCount int        `json:"ticket_count,omitempty"`
	Color       string     `json:"color,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	Tickets     []Ticket   `json:"tickets,omitempty"`
}
