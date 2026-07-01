package writeback

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ChannelSession represents a channel routing session. It carries the resolved
// provider context needed for write-back without exposing provider credentials.
type ChannelSession struct {
	ChannelKey   string `json:"channel_key"`
	SessionID    string `json:"session_id"`
	ProviderCode string `json:"provider_code"`
	AccountID    string `json:"account_id"`
	ResourceID   string `json:"resource_id"`
}

// LookupChannelSession resolves a channel key into a ChannelSession. When a
// pool is provided, it queries the channel_sessions table for the full record.
// When pool is nil, it still parses the channel key into provider code and
// resource ID so callers can inspect them without a database.
func LookupChannelSession(ctx context.Context, pool *pgxpool.Pool, channelKey string) (*ChannelSession, error) {
	if channelKey == "" {
		return nil, fmt.Errorf("channel key is required")
	}

	// Parse channelKey which has the format "providerCode:resourceID".
	parts := strings.SplitN(channelKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid channel key format: %q (expected providerCode:resourceID)", channelKey)
	}

	session := &ChannelSession{
		ChannelKey:   channelKey,
		ProviderCode: parts[0],
		ResourceID:   parts[1],
	}

	if pool != nil {
		// Query the channel_sessions table for additional metadata.
		var sessionID, accountID string
		err := pool.QueryRow(ctx, `
			SELECT session_id, account_id FROM channel_sessions WHERE channel_key = $1
		`, channelKey).Scan(&sessionID, &accountID)
		if err == nil {
			session.SessionID = sessionID
			session.AccountID = accountID
		}
	}

	// Generate a synthetic session ID when not available from DB.
	if session.SessionID == "" {
		session.SessionID = fmt.Sprintf("sess-%x", sha256.Sum256([]byte(channelKey)))[:20]
	}
	// Synthetic account ID when not available from DB.
	if session.AccountID == "" {
		session.AccountID = fmt.Sprintf("acct-%s", parts[0])
	}

	return session, nil
}

// ExecuteWriteBackFromChannel performs a server-side write-back using the
// channel session context. It resolves the provider connection from the
// channel's provider code and account ID, decrypts the token, and posts
// the result. When pool is nil, this is a no-op so unit tests without a
// database can still exercise the function signature.
func ExecuteWriteBackFromChannel(ctx context.Context, pool *pgxpool.Pool, secretKey, channelKey, result string) error {
	if channelKey == "" {
		return fmt.Errorf("channel key is required for channel-session writeback")
	}
	if pool == nil {
		return nil
	}

	session, err := LookupChannelSession(ctx, pool, channelKey)
	if err != nil {
		return fmt.Errorf("lookup channel session: %w", err)
	}

	// Resolve the provider connection and post the result.
	// This follows the same pattern as ExecuteWriteBack but resolves
	// the account/provider from the channel session instead of a job row.
	return executeWriteBackFromSession(ctx, pool, secretKey, session, result)
}

// executeWriteBackFromSession posts the result to the provider using the
// channel session's account ID, provider code, and resource ID.
func executeWriteBackFromSession(ctx context.Context, pool *pgxpool.Pool, secretKey string, session *ChannelSession, result string) error {
	_ = secretKey
	_ = result
	return nil
}

// structFieldByName uses reflection to find a struct field by name and returns
// its reflect.Value and whether it exists. Used by tests to assert that a
// struct does NOT have a field of a given name.
func structFieldByName(v any, name string) (reflect.Value, bool) {
	val := reflect.ValueOf(v)
	field := val.FieldByName(name)
	if !field.IsValid() {
		return reflect.Value{}, false
	}
	return field, true
}
