package catalog

import (
	"testing"

	"mework/libs/shared/grant"
)

func TestGrant_Permits(t *testing.T) {
	t.Parallel()

	key := []byte("test-key-for-grant-permits")
	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpRepoRead}, key)
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	tests := []struct {
		name string
		op   grant.Operation
		want bool
	}{
		{"permits OpPullAgent", grant.OpPullAgent, true},
		{"permits OpRepoRead", grant.OpRepoRead, true},
		{"denies OpRepoWrite", grant.OpRepoWrite, false},
		{"denies OpNetwork", grant.OpNetwork, false},
		{"denies OpSpawn", grant.OpSpawn, false},
		{"denies OpWriteBack", grant.OpWriteBack, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.Permits(tt.op)
			if got != tt.want {
				t.Errorf("Permits(%v) = %v, want %v", tt.op, got, tt.want)
			}
		})
	}
}

func TestGrant_SignVerify(t *testing.T) {
	t.Parallel()

	key := []byte("test-key-for-sign-verify")

	// Create a signed grant.
	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpRepoRead}, key)
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	// Verify the original grant succeeds.
	if err := grant.VerifyGrant(g, key); err != nil {
		t.Errorf("VerifyGrant should succeed for untampered grant: %v", err)
	}

	// Tamper: add an extra operation.
	g.Ops = append(g.Ops, grant.OpSpawn)
	if err := grant.VerifyGrant(g, key); err == nil {
		t.Error("VerifyGrant should fail for tampered grant (added ops)")
	}

	// Tamper: change scope.
	g2, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, key)
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	g2.Scope = map[string]string{"repo": "other"}
	if err := grant.VerifyGrant(g2, key); err == nil {
		t.Error("VerifyGrant should fail for tampered grant (changed scope)")
	}
}

func TestGrant_PerRunScoping(t *testing.T) {
	t.Parallel()

	key := []byte("test-key-per-run-scoping")

	gBroad, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent, grant.OpRepoRead, grant.OpRepoWrite}, key)
	if err != nil {
		t.Fatalf("NewGrant broad: %v", err)
	}

	gMinimal, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, key)
	if err != nil {
		t.Fatalf("NewGrant minimal: %v", err)
	}

	// Broad grant permits all three.
	if !gBroad.Permits(grant.OpPullAgent) {
		t.Error("gBroad should permit OpPullAgent")
	}
	if !gBroad.Permits(grant.OpRepoRead) {
		t.Error("gBroad should permit OpRepoRead")
	}
	if !gBroad.Permits(grant.OpRepoWrite) {
		t.Error("gBroad should permit OpRepoWrite")
	}

	// Minimal grant permits only OpPullAgent.
	if !gMinimal.Permits(grant.OpPullAgent) {
		t.Error("gMinimal should permit OpPullAgent")
	}
	if gMinimal.Permits(grant.OpRepoRead) {
		t.Error("gMinimal should NOT permit OpRepoRead")
	}
	if gMinimal.Permits(grant.OpRepoWrite) {
		t.Error("gMinimal should NOT permit OpRepoWrite")
	}

	// Minimal grant also denies unrelated ops.
	if gMinimal.Permits(grant.OpNetwork) {
		t.Error("gMinimal should NOT permit OpNetwork")
	}
}
