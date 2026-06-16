package token

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateRandomToken generates a random 256-bit token prefixed with "rt_"
func GenerateRandomToken() (string, error) {
	b := make([]byte, 32) // 256 bits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "rt_" + hex.EncodeToString(b), nil
}

// ComputeLookup computes the HMAC-SHA256 lookup hash for a token using the server key
func ComputeLookup(token string, serverKey string) string {
	mac := hmac.New(sha256.New, []byte(serverKey))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
