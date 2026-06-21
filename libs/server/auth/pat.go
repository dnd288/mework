package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/shared/providers/mello"
)

type contextKey string

const (
	AccountIDKey contextKey = "account_id"
	PATTokenKey  contextKey = "pat_token"
	TenantIDKey  contextKey = "tenant_id"
)

// GetAccountID retrieves the account ID from the request context.
func GetAccountID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AccountIDKey).(string)
	return val, ok
}

// GetTenantID retrieves the authenticated PAT's tenant from the request context.
func GetTenantID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(TenantIDKey).(string)
	return val, ok
}

// GetPATToken retrieves the PAT token from the request context.
func GetPATToken(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(PATTokenKey).(string)
	return val, ok
}

type cacheEntry struct {
	accountID string
	expiry    time.Time
	err       error
}

// PATAuthenticator handles authenticating Personal Access Tokens (PATs) against the Mello API,
// resolving and caching the associated account in the database.
type PATAuthenticator struct {
	Pool         *pgxpool.Pool
	MelloBaseURL string
	cache        sync.Map
	TTL          time.Duration
}

// NewPATAuthenticator creates a new PATAuthenticator.
func NewPATAuthenticator(pool *pgxpool.Pool, melloBaseURL string) *PATAuthenticator {
	return &PATAuthenticator{
		Pool:         pool,
		MelloBaseURL: melloBaseURL,
		TTL:          60 * time.Second,
	}
}

// Middleware returns a chi-compatible middleware handler.
func (a *PATAuthenticator) Middleware(next http.Handler) http.Handler {
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

		patToken := parts[1]
		if patToken == "" {
			http.Error(w, "Unauthorized: empty token", http.StatusUnauthorized)
			return
		}

		accountID, err := a.resolveAccount(r.Context(), patToken)
		if err != nil {
			var apiErr *mello.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}
			log.Printf("PAT authentication error: %v", err)
			http.Error(w, "Service Unavailable: authentication failed", http.StatusServiceUnavailable)
			return
		}

		tenantID, err := a.resolveTenant(r.Context(), accountID)
		if err != nil {
			log.Printf("PAT tenant resolution error: %v", err)
			http.Error(w, "Service Unavailable: authentication failed", http.StatusServiceUnavailable)
			return
		}

		ctx := context.WithValue(r.Context(), AccountIDKey, accountID)
		ctx = context.WithValue(ctx, PATTokenKey, patToken)
		ctx = context.WithValue(ctx, TenantIDKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveTenant returns the tenant that owns the account's Mello identity. The
// authenticated credential is thereby bound to its tenant, so authorized requests can
// be scoped and cross-tenant access denied.
func (a *PATAuthenticator) resolveTenant(ctx context.Context, accountID string) (string, error) {
	var tenantID string
	err := a.Pool.QueryRow(ctx, `
		SELECT tenant_id FROM account_identities
		WHERE account_id = $1 AND provider_code = $2
	`, accountID, "mello").Scan(&tenantID)
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

func (a *PATAuthenticator) resolveAccount(ctx context.Context, token string) (string, error) {
	tokenHash := a.hashToken(token)

	// Check cache
	if val, ok := a.cache.Load(tokenHash); ok {
		entry := val.(cacheEntry)
		if time.Now().Before(entry.expiry) {
			if entry.err != nil {
				return "", entry.err
			}
			return entry.accountID, nil
		}
		a.cache.Delete(tokenHash)
	}

	// Cache miss: resolve account from Mello API + Database
	accountID, err := a.fetchAndUpsert(ctx, token)
	if err != nil {
		// Cache negative hits for 401s
		var apiErr *mello.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			a.cache.Store(tokenHash, cacheEntry{
				err:    err,
				expiry: time.Now().Add(a.TTL),
			})
		}
		return "", err
	}

	// Cache positive hit
	a.cache.Store(tokenHash, cacheEntry{
		accountID: accountID,
		expiry:    time.Now().Add(a.TTL),
	})

	return accountID, nil
}

func (a *PATAuthenticator) hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (a *PATAuthenticator) fetchAndUpsert(ctx context.Context, token string) (string, error) {
	// 1. Call Mello /me to get external user details
	client := mello.NewClient(a.MelloBaseURL, token, 10*time.Second, "mework-server")
	user, err := client.GetCurrentUser()
	if err != nil {
		return "", err
	}

	// 2. Query database for existing identity
	var accountID string
	err = a.Pool.QueryRow(ctx, `
		SELECT account_id FROM account_identities
		WHERE provider_code = $1 AND external_user_id = $2
	`, "mello", user.ID).Scan(&accountID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Identity does not exist, create new account and identity in a transaction
			accountName := user.Name
			if accountName == "" {
				accountName = user.Email
			}
			if accountName == "" {
				accountName = "Mello User " + user.ID
			}

			tx, txErr := a.Pool.Begin(ctx)
			if txErr != nil {
				return "", txErr
			}
			defer tx.Rollback(ctx)

			txErr = tx.QueryRow(ctx, `
				INSERT INTO accounts (name) VALUES ($1) RETURNING id
			`, accountName).Scan(&accountID)
			if txErr != nil {
				return "", txErr
			}

			_, txErr = tx.Exec(ctx, `
				INSERT INTO account_identities (account_id, provider_code, external_user_id)
				VALUES ($1, $2, $3)
			`, accountID, "mello", user.ID)
			if txErr != nil {
				return "", txErr
			}

			if txErr = tx.Commit(ctx); txErr != nil {
				return "", txErr
			}
		} else {
			return "", err
		}
	}

	// 3. Sync watched containers (boards) outside of main transaction
	go func() {
		// Run with background context to ensure it doesn't get cancelled by the client request timeout
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := a.syncWatchedContainers(bgCtx, client, accountID); err != nil {
			log.Printf("Failed to sync watched containers for account %s: %v", accountID, err)
		}
	}()

	return accountID, nil
}

func (a *PATAuthenticator) syncWatchedContainers(ctx context.Context, client *mello.Client, accountID string) error {
	workspaces, err := client.ListWorkspaces()
	if err != nil {
		return err
	}

	var boards []mello.Board
	for _, ws := range workspaces {
		wsBoards, err := client.ListWorkspaceBoards(ws.ID)
		if err != nil {
			return err
		}
		boards = append(boards, wsBoards...)
	}

	if len(boards) == 0 {
		return nil
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, b := range boards {
		_, err = tx.Exec(ctx, `
			INSERT INTO watched_containers (account_id, provider_code, external_container_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (provider_code, external_container_id) DO UPDATE SET account_id = EXCLUDED.account_id
		`, accountID, "mello", b.ID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
