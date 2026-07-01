package registry

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPresenceHandler returns an HTTP handler that updates a runtime's status
// (online/offline) or refreshes its heartbeat. Called by the daemon for
// presence management via POST /api/v1/runners/{id}/{online,offline,heartbeat}.
func NewPresenceHandler(pool *pgxpool.Pool, status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runnerID := r.PathValue("id")
		if runnerID == "" {
			http.Error(w, "missing runner id", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_, err := pool.Exec(ctx, `
			UPDATE runtimes
			SET last_seen_at = NOW(), status = $1
			WHERE id = $2
		`, status, runnerID)
		if err != nil {
			log.Printf("presence update error: %v", err)
			http.Error(w, fmt.Sprintf("presence update error: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
