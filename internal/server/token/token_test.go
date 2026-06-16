package token

import (
	"strings"
	"testing"
)

func TestGenerateRandomToken(t *testing.T) {
	tok1, err := GenerateRandomToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok2, err := GenerateRandomToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tok1 == tok2 {
		t.Errorf("expected generated tokens to be unique, got duplicate: %s", tok1)
	}

	if !strings.HasPrefix(tok1, "rt_") {
		t.Errorf("expected token to have 'rt_' prefix, got: %s", tok1)
	}

	// 32 bytes in hex = 64 characters. With "rt_" it should be 67 characters.
	if len(tok1) != 67 {
		t.Errorf("expected token length of 67, got %d for token %s", len(tok1), tok1)
	}
}

func TestComputeLookup(t *testing.T) {
	token := "rt_dummy_token_value"
	key1 := "secret_key_1"
	key2 := "secret_key_2"

	lookup1 := ComputeLookup(token, key1)
	lookup2 := ComputeLookup(token, key2)

	if lookup1 == lookup2 {
		t.Errorf("expected different lookups for different keys, got identical hash: %s", lookup1)
	}

	lookup1Repeat := ComputeLookup(token, key1)
	if lookup1 != lookup1Repeat {
		t.Errorf("expected lookup computation to be deterministic, got %s and %s", lookup1, lookup1Repeat)
	}
}
