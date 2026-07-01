package runner

import (
	"encoding/json"
	"fmt"

	"mework/libs/shared/grant"
)

// parseAndVerifyGrant unmarshals a JSON grant and verifies its HMAC signature
// using the provided key. Returns the parsed grant on success.
func parseAndVerifyGrant(raw json.RawMessage, key []byte) (*grant.Grant, error) {
	var g grant.Grant
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("parse grant: %w", err)
	}
	if err := grant.VerifyGrant(&g, key); err != nil {
		return nil, fmt.Errorf("verify grant: %w", err)
	}
	return &g, nil
}

// enforceGrant checks that the grant permits the given operation. Returns an
// error if the operation is not permitted (least-privilege: absent means denied).
func enforceGrant(g *grant.Grant, op grant.Operation) error {
	if !g.Permits(op) {
		return fmt.Errorf("operation %q not permitted by grant", op)
	}
	return nil
}
