package secret

import (
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := "some_strong_development_secret_key"
	plaintext := "my_precious_pat_token_value_123"

	ciphertext, err := Seal(plaintext, key)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Errorf("expected ciphertext to be encrypted, got plaintext: %s", ciphertext)
	}

	decrypted, err := Open(ciphertext, key)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected decrypted plaintext to match, got %s, expected %s", decrypted, plaintext)
	}
}

func TestSealOpenWrongKey(t *testing.T) {
	key1 := "key_number_one"
	key2 := "key_number_two"
	plaintext := "sensitive data"

	ciphertext, err := Seal(plaintext, key1)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	_, err = Open(ciphertext, key2)
	if err == nil {
		t.Error("expected decryption to fail with a different key, but it succeeded")
	}
}

func TestEmptyKey(t *testing.T) {
	_, err := Seal("data", "")
	if err == nil {
		t.Error("expected Seal with empty key to return error")
	}

	_, err = Open("data", "")
	if err == nil {
		t.Error("expected Open with empty key to return error")
	}
}
