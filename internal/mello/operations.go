package mello

import (
	"fmt"
	"net/url"
)

// --- Reads ---

// GetCurrentUser returns the token owner (GET /me).
func (c *Client) GetCurrentUser() (*User, error) {
	out := &User{}
	return out, c.do("GET", "/me", true, nil, out)
}

// ListWorkspaces returns token-accessible workspaces.
func (c *Client) ListWorkspaces() ([]Workspace, error) {
	var out []Workspace
	return out, c.do("GET", "/workspaces", true, nil, &out)
}

// ListWorkspaceMembers returns members of a workspace.
func (c *Client) ListWorkspaceMembers(workspaceID string) ([]WorkspaceMember, error) {
	var out []WorkspaceMember
	return out, c.do("GET", "/workspaces/"+workspaceID+"/members", true, nil, &out)
}

// ListWorkspaceBoards returns boards in a workspace.
func (c *Client) ListWorkspaceBoards(workspaceID string) ([]Board, error) {
	var out []Board
	return out, c.do("GET", "/workspaces/"+workspaceID+"/boards", true, nil, &out)
}

// GetBoard returns a board with columns and tickets.
func (c *Client) GetBoard(boardID string) (*Board, error) {
	out := &Board{}
	return out, c.do("GET", "/boards/"+boardID, true, nil, out)
}

// ListColumns returns a board's columns.
func (c *Client) ListColumns(boardID string) ([]Column, error) {
	var out []Column
	return out, c.do("GET", "/boards/"+boardID+"/columns", true, nil, &out)
}

// ListBoardTickets returns all tickets on a board.
func (c *Client) ListBoardTickets(boardID string) ([]Ticket, error) {
	var out []Ticket
	return out, c.do("GET", "/boards/"+boardID+"/tickets", true, nil, &out)
}

// GetTicket returns a ticket with comments, activities and checklists.
func (c *Client) GetTicket(ticketID string) (*TicketDetail, error) {
	out := &TicketDetail{}
	return out, c.do("GET", "/tickets/"+ticketID, true, nil, out)
}

// ListComments returns a ticket's comments.
func (c *Client) ListComments(ticketID string) ([]Comment, error) {
	var out []Comment
	return out, c.do("GET", "/tickets/"+ticketID+"/comments", true, nil, &out)
}

// ListHistory returns a ticket's history entries.
func (c *Client) ListHistory(ticketID string) ([]HistoryEntry, error) {
	var out []HistoryEntry
	return out, c.do("GET", "/tickets/"+ticketID+"/history", true, nil, &out)
}

// SearchTickets runs a full-text search within a workspace.
func (c *Client) SearchTickets(workspaceID, q string) ([]SearchResult, error) {
	var out []SearchResult
	path := fmt.Sprintf("/search?workspace_id=%s&q=%s", url.QueryEscape(workspaceID), url.QueryEscape(q))
	return out, c.do("GET", path, true, nil, &out)
}

// --- Writes (REST CRUD; daemon write-back uses the MCP client instead) ---

// CreateComment posts a comment to a ticket.
func (c *Client) CreateComment(ticketID, body string) (*Comment, error) {
	out := &Comment{}
	return out, c.do("POST", "/tickets/"+ticketID+"/comments", true, map[string]any{"body": body}, out)
}

// CreateTicket creates a ticket in a column. updates carries title (required)
// plus optional description/position.
func (c *Client) CreateTicket(columnID string, updates map[string]any) (*Ticket, error) {
	out := &Ticket{}
	return out, c.do("POST", "/columns/"+columnID+"/tickets", true, updates, out)
}

// MoveTicket moves a ticket to another column/position.
func (c *Client) MoveTicket(ticketID, columnID string, position int) (*MoveTicketResult, error) {
	out := &MoveTicketResult{}
	payload := map[string]any{"column_id": columnID, "position": position}
	return out, c.do("PATCH", "/tickets/"+ticketID+"/move", true, payload, out)
}

// UpdateTicket patches mutable ticket fields. Only non-nil entries in updates
// are sent, matching the SDK's UNSET semantics.
func (c *Client) UpdateTicket(ticketID string, updates map[string]any) (*Ticket, error) {
	out := &Ticket{}
	return out, c.do("PATCH", "/tickets/"+ticketID, true, updates, out)
}

// CreateChecklist adds a checklist to a ticket (non-v1 path).
func (c *Client) CreateChecklist(ticketID, title string) (*Checklist, error) {
	out := &Checklist{}
	return out, c.do("POST", "/tickets/"+ticketID+"/checklists", false, map[string]any{"title": title}, out)
}

// CreateChecklistItem adds an item to a checklist (non-v1 path).
func (c *Client) CreateChecklistItem(checklistID, title string) (*ChecklistItem, error) {
	out := &ChecklistItem{}
	return out, c.do("POST", "/checklists/"+checklistID+"/items", false, map[string]any{"title": title}, out)
}

// UpdateChecklistItem toggles a checklist item's checked state (non-v1 path).
func (c *Client) UpdateChecklistItem(itemID string, isChecked bool) (*ChecklistItem, error) {
	out := &ChecklistItem{}
	return out, c.do("PATCH", "/checklist-items/"+itemID, false, map[string]any{"is_checked": isChecked}, out)
}
