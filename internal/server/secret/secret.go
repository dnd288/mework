package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// Seal encrypts a plaintext string using AES-256-GCM with a key derived from the secret key.
// It returns a hex-encoded string containing the ciphertext prepended with the nonce.
func Seal(plaintext string, key string) (string, error) {
	if key == "" {
		return "", errors.New("empty encryption key")
	}

	// Derive a 32-byte key using SHA-256
	h := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Open decrypts a hex-encoded ciphertext (which includes the nonce at the beginning) using AES-256-GCM.
func Open(ciphertextHex string, key string) (string, error) {
	if key == "" {
		return "", errors.New("empty decryption key")
	}

	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", err
	}

	h := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
