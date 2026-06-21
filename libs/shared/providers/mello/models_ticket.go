package mello

import "time"

// Ticket is a card on a board. AssigneeID, CreatedAt and the comment list
// drive the daemon's trigger logic.
type Ticket struct {
	ID                    string         `json:"id"`
	TicketNumber          int            `json:"ticket_number"`
	TicketCode            string         `json:"ticket_code"`
	ColumnID              string         `json:"column_id"`
	Title                 string         `json:"title"`
	Description           string         `json:"description"`
	DescriptionHTML       string         `json:"description_html"`
	Position              int            `json:"position"`
	BoardCode             string         `json:"board_code,omitempty"`
	AssigneeID            string         `json:"assignee_id,omitempty"`
	StartDate             *time.Time     `json:"start_date,omitempty"`
	EndDate               *time.Time     `json:"end_date,omitempty"`
	CreatedAt             *time.Time     `json:"created_at,omitempty"`
	UpdatedAt             *time.Time     `json:"updated_at,omitempty"`
	Labels                []Label        `json:"labels,omitempty"`
	Members               []TicketMember `json:"members,omitempty"`
	CommentCount          int            `json:"comment_count,omitempty"`
	AttachmentCount       int            `json:"attachment_count,omitempty"`
	ChecklistItemCount    int            `json:"checklist_item_count,omitempty"`
	ChecklistCheckedCount int            `json:"checklist_checked_count,omitempty"`
}

// TicketDetail extends Ticket with nested collections returned by GET /tickets/{id}.
type TicketDetail struct {
	Ticket
	BoardID     string         `json:"board_id,omitempty"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	ColumnName  string         `json:"column_name,omitempty"`
	Comments    []Comment      `json:"comments,omitempty"`
	Activities  []HistoryEntry `json:"activities,omitempty"`
	Checklists  []Checklist    `json:"checklists,omitempty"`
	Attachments []Attachment   `json:"attachments,omitempty"`
}

// Comment is a message on a ticket. UserID/Author identify the writer so the
// daemon can skip its own comments when scanning for the trigger keyword.
type Comment struct {
	ID        string     `json:"id"`
	TicketID  string     `json:"ticket_id"`
	UserID    string     `json:"user_id"`
	Body      string     `json:"body"`
	BodyHTML  string     `json:"body_html"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Author    *User      `json:"author,omitempty"`
}

// HistoryEntry is an activity log record on a ticket.
type HistoryEntry struct {
	ID          string         `json:"id"`
	TicketID    string         `json:"ticket_id"`
	WorkspaceID string         `json:"workspace_id"`
	Action      string         `json:"action"`
	UserID      string         `json:"user_id,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	CreatedAt   *time.Time     `json:"created_at,omitempty"`
	Author      *User          `json:"author,omitempty"`
}

// Checklist groups checklist items on a ticket.
type Checklist struct {
	ID        string          `json:"id"`
	TicketID  string          `json:"ticket_id"`
	Title     string          `json:"title"`
	Position  int             `json:"position"`
	CreatedAt *time.Time      `json:"created_at,omitempty"`
	UpdatedAt *time.Time      `json:"updated_at,omitempty"`
	Items     []ChecklistItem `json:"items,omitempty"`
}

// ChecklistItem is a single checkable entry within a checklist.
type ChecklistItem struct {
	ID          string     `json:"id"`
	ChecklistID string     `json:"checklist_id"`
	Title       string     `json:"title"`
	IsChecked   bool       `json:"is_checked"`
	Position    int        `json:"position"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// Attachment is a file uploaded to a ticket.
type Attachment struct {
	ID          string     `json:"id"`
	TicketID    string     `json:"ticket_id"`
	UserID      string     `json:"user_id"`
	Bucket      string     `json:"bucket"`
	ObjectKey   string     `json:"object_key"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	ByteSize    int64      `json:"byte_size"`
	ETag        string     `json:"etag"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Author      *User      `json:"author,omitempty"`
}

// SearchResult is a ticket match from the search endpoint.
type SearchResult struct {
	ID           string     `json:"id"`
	TicketNumber int        `json:"ticket_number"`
	TicketCode   string     `json:"ticket_code"`
	BoardCode    string     `json:"board_code"`
	ColumnID     string     `json:"column_id"`
	BoardID      string     `json:"board_id"`
	WorkspaceID  string     `json:"workspace_id"`
	BoardName    string     `json:"board_name"`
	ColumnName   string     `json:"column_name"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Rank         float64    `json:"rank"`
	AssigneeID   string     `json:"assignee_id,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

// MoveTicketResult is returned by the move endpoint.
type MoveTicketResult struct {
	Ticket      Ticket `json:"ticket"`
	WorkspaceID string `json:"workspace_id"`
	FromColumn  string `json:"from_column"`
	ToColumn    string `json:"to_column"`
}
