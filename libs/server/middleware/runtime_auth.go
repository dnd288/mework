package middleware

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/platform/token"
)

type contextKey string

const (
	RuntimeIDKey contextKey = "runtime_id"
	AccountIDKey contextKey = "account_id"
	TenantIDKey  contextKey = "tenant_id"
)

func GetRuntimeID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(RuntimeIDKey).(string)
	return val, ok
}

func GetAccountID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AccountIDKey).(string)
	return val, ok
}

// GetTenantID retrieves the authenticated credential's tenant from the request context.
func GetTenantID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(TenantIDKey).(string)
	return val, ok
}

// RuntimeAuthenticator authenticates runtimes using the rt_xxx Bearer tokens.
type RuntimeAuthenticator struct {
	pool      *pgxpool.Pool
	serverKey string
}

func NewRuntimeAuthenticator(pool *pgxpool.Pool, serverKey string) *RuntimeAuthenticator {
	return &RuntimeAuthenticator{
		pool:      pool,
		serverKey: serverKey,
	}
}

func (a *RuntimeAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "Unauthorized: invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		rtToken := parts[1]
		if rtToken == "" {
			http.Error(w, "Unauthorized: empty token", http.StatusUnauthorized)
			return
		}

		// MEWORK_DEV=1 bypasses runtime token DB lookup for local development.
		if os.Getenv("MEWORK_DEV") == "1" {
			devRuntimeID := os.Getenv("MEWORK_DEV_RUNTIME")
			if devRuntimeID == "" {
				devRuntimeID = "dev-runtime"
			}
			devAccountID := os.Getenv("MEWORK_DEV_ACCOUNT")
			if devAccountID == "" {
				devAccountID = "dev-account"
			}
			devTenantID := os.Getenv("MEWORK_DEV_TENANT")
			if devTenantID == "" {
				devTenantID = "00000000-0000-0000-0000-000000000001"
			}
			ctx := context.WithValue(r.Context(), RuntimeIDKey, devRuntimeID)
			ctx = context.WithValue(ctx, AccountIDKey, devAccountID)
			ctx = context.WithValue(ctx, TenantIDKey, devTenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Compute lookup
		lookup := token.ComputeLookup(rtToken, a.serverKey)

		var runtimeID, accountID, tenantID string
		err := a.pool.QueryRow(r.Context(), `
			SELECT id, account_id, tenant_id FROM runtimes
			WHERE token_lookup = $1
		`, lookup).Scan(&runtimeID, &accountID, &tenantID)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "Unauthorized: invalid runtime token", http.StatusUnauthorized)
				return
			}
			log.Printf("Runtime auth database error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Update runtime last_seen_at and set status to online
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = a.pool.Exec(bgCtx, `
				UPDATE runtimes
				SET last_seen_at = NOW(), status = 'online'
				WHERE id = $1
			`, runtimeID)
		}()

		ctx := context.WithValue(r.Context(), RuntimeIDKey, runtimeID)
		ctx = context.WithValue(ctx, AccountIDKey, accountID)
		ctx = context.WithValue(ctx, TenantIDKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
