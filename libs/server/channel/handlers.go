package channel

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/auth"
)

// Handlers provides HTTP handlers for channel-related endpoints.
type Handlers struct {
	pool *pgxpool.Pool
}

// NewHandlers creates a new Handlers with the given pool.
func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
}

// channelSession is the JSON response for a channel session listing.
type channelSession struct {
	ChannelKey   string     `json:"channel_key"`
	SessionID    string     `json:"session_id"`
	ProviderCode string     `json:"provider_code"`
	ResourceID   string     `json:"resource_id"`
	RunnerID     string     `json:"runner_id"`
	Spec         string     `json:"spec"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

// ListChannels handles GET /api/v1/channels. It returns all active channel
// sessions scoped to the authenticated tenant.
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized: tenant not found", http.StatusUnauthorized)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT cs.channel_key, cs.session_id, cs.provider_code, cs.resource_id,
		       cs.runner_id, cs.spec, cs.status, cs.created_at, cs.closed_at
		FROM channel_sessions cs
		JOIN runtimes r ON cs.runner_id = r.id::text
		WHERE r.tenant_id = $1
		ORDER BY cs.created_at DESC
	`, tenantID)
	if err != nil {
		http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sessions []channelSession
	for rows.Next() {
		var s channelSession
		if err := rows.Scan(&s.ChannelKey, &s.SessionID, &s.ProviderCode, &s.ResourceID,
			&s.RunnerID, &s.Spec, &s.Status, &s.CreatedAt, &s.ClosedAt); err != nil {
			http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []channelSession{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}
