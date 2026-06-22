package hub

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LivenessHandler reports process liveness. It is independent of the database
// so a transient DB blip does not flap liveness and trigger restarts.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// ReadinessHandler reports whether the server can serve traffic — i.e. the
// database is reachable. It returns 503 when not ready, with a generic body
// (the underlying DB error is logged server-side, never leaked to the caller).
func ReadinessHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if pool == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not ready"}`))
			return
		}

		// Short timeout so a readiness probe never hangs.
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			log.Printf("readiness: database unreachable: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not ready"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// HealthHandler is retained for backward compatibility (the `/healthz` route).
// It mirrors readiness — 200 when the DB is reachable, 503 otherwise — without
// leaking the underlying error.
func HealthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return ReadinessHandler(pool)
}
