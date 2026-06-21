package runner

import (
	"encoding/json"
	"testing"

	"mework/libs/shared/grant"
)

func TestGrant_ParseAndVerify(t *testing.T) {
	key := []byte("test-key-32-bytes-long-for-hmac!!")

	// Create a signed grant.
	g, err := grant.NewGrant([]grant.Operation{grant.OpRepoRead, grant.OpPullAgent}, key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(g)

	// Should parse and verify successfully.
	parsed, err := parseAndVerifyGrant(raw, key)
	if err != nil {
		t.Fatalf("expected valid grant to pass verification: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected non-nil grant")
	}
	if !parsed.Permits(grant.OpRepoRead) {
		t.Error("grant should permit OpRepoRead")
	}

	// Tamper: modify the ops and re-marshal (but don't re-sign).
	g2 := new(grant.Grant)
	if err := json.Unmarshal(raw, g2); err != nil {
		t.Fatal(err)
	}
	g2.Ops = append(g2.Ops, grant.OpNetwork)
	tampered, _ := json.Marshal(g2)

	_, err = parseAndVerifyGrant(tampered, key)
	if err == nil {
		t.Error("expected error for tampered grant")
	}
}

func TestGrant_OperationProceeds(t *testing.T) {
	key := []byte("test-key-32-bytes-long-for-hmac!!")
	g, err := grant.NewGrant([]grant.Operation{grant.OpRepoRead, grant.OpPullAgent, grant.OpSpawn}, key)
	if err != nil {
		t.Fatal(err)
	}

	if err := enforceGrant(g, grant.OpRepoRead); err != nil {
		t.Errorf("OpRepoRead should be permitted: %v", err)
	}
	if err := enforceGrant(g, grant.OpPullAgent); err != nil {
		t.Errorf("OpPullAgent should be permitted: %v", err)
	}
}

func TestGrant_OperationRefused(t *testing.T) {
	key := []byte("test-key-32-bytes-long-for-hmac!!")
	g, err := grant.NewGrant([]grant.Operation{grant.OpRepoRead, grant.OpPullAgent}, key)
	if err != nil {
		t.Fatal(err)
	}

	// In-grant ops should proceed.
	if err := enforceGrant(g, grant.OpRepoRead); err != nil {
		t.Errorf("OpRepoRead should be permitted: %v", err)
	}

	// Out-grant ops should be refused.
	if err := enforceGrant(g, grant.OpNetwork); err == nil {
		t.Error("OpNetwork should be refused (not in grant)")
	}
	if err := enforceGrant(g, grant.OpRepoWrite); err == nil {
		t.Error("OpRepoWrite should be refused (not in grant)")
	}
	if err := enforceGrant(g, grant.OpSpawn); err == nil {
		t.Error("OpSpawn should be refused (not in grant)")
	}
}

func TestGrant_NoWidening(t *testing.T) {
	key1 := []byte("key-1-for-signing-grants-00001")
	key2 := []byte("key-2-for-signing-grants-00002")

	// Create a grant signed with key1.
	g, err := grant.NewGrant([]grant.Operation{grant.OpRepoRead}, key1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(g)

	// Verify with key1 should pass.
	if _, err := parseAndVerifyGrant(raw, key1); err != nil {
		t.Fatalf("verify with original key should pass: %v", err)
	}

	// Tamper: modify ops and re-sign with a different key (simulating runner
	// trying to widen its scope with a key it controls).
	g2 := new(grant.Grant)
	json.Unmarshal(raw, g2)
	g2.Ops = append(g2.Ops, grant.OpNetwork)
	widened, _ := grant.NewGrant(g2.Ops, key2)
	rawWidened, _ := json.Marshal(widened)

	// Verify with the original key1 should fail — the runner does not have
	// key1, so it cannot produce a valid signature for a widened grant.
	if _, err := parseAndVerifyGrant(rawWidened, key1); err == nil {
		t.Error("tampered and re-signed grant with different key should fail verification with original key")
	}

	// The runner cannot widen by just modifying the grant JSON without
	// re-signing: the original signature no longer matches the tampered
	// payload.
	var modified grant.Grant
	json.Unmarshal(raw, &modified)
	modified.Ops = append(modified.Ops, grant.OpNetwork)
	// Sig is the original valid signature for [OpRepoRead], which no longer
	// matches the json of [OpRepoRead, OpNetwork].
	rawModified, _ := json.Marshal(modified)
	if _, err := parseAndVerifyGrant(rawModified, key1); err == nil {
		t.Error("tampered grant should fail verification - signature mismatch")
	}
}
