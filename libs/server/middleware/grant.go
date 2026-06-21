package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"mework/libs/shared/grant"
)

const (
	grantKey contextKey = "grant"
)

// GetGrant retrieves the verified grant from the request context.
func GetGrant(ctx context.Context) (*grant.Grant, bool) {
	g, ok := ctx.Value(grantKey).(*grant.Grant)
	return g, ok
}

// GrantMiddleware returns a chi middleware that reads a grant from the
// X-Grant request header, verifies its integrity via grant.VerifyGrant,
// and injects the verified grant into the request context.
func GrantMiddleware(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			grantHeader := r.Header.Get("X-Grant")
			if grantHeader == "" {
				// No grant header — pass through without a grant in context.
				// RequireOperation downstream will deny the request.
				next.ServeHTTP(w, r)
				return
			}

			var g grant.Grant
			if err := json.Unmarshal([]byte(grantHeader), &g); err != nil {
				http.Error(w, "Bad Request: invalid grant", http.StatusBadRequest)
				return
			}

			if err := grant.VerifyGrant(&g, key); err != nil {
				// Tampered or invalid signature: deny (GRANT-01).
				http.Error(w, "Forbidden: grant verification failed", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), grantKey, &g)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireOperation returns a chi middleware that checks the grant in the
// request context permits the given operation. Returns 403 if no grant is
// present or the operation is not permitted.
func RequireOperation(op grant.Operation) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			g, ok := GetGrant(r.Context())
			if !ok {
				http.Error(w, "Forbidden: grant required", http.StatusForbidden)
				return
			}
			if !g.Permits(op) {
				http.Error(w, "Forbidden: grant does not permit operation", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
