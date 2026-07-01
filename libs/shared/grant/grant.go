// Package grant provides cryptographic sign/verify primitives for
// permission grants that authenticate agent↔resource access.
package grant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"
)

// Sign creates an HMAC-SHA256 signature of data using the provided key.
func Sign(data, key []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil), nil
}

// Verify checks whether sig is a valid HMAC-SHA256 signature of data
// under the given key.
func Verify(sig, data, key []byte) bool {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

// Operation is a named permission that a grant may convey.
type Operation string

const (
	OpPullAgent   Operation = "agent.pull"
	OpSpawn       Operation = "agent.spawn"
	OpRepoRead    Operation = "repo.read"
	OpRepoWrite   Operation = "repo.write"
	OpNetwork     Operation = "network"
	OpWriteBack   Operation = "writeback"
	OpWorkspaceRead  Operation = "workspace.read"
	OpWorkspaceWrite Operation = "workspace.write"
	OpWorkspacePush  Operation = "workspace.push"
)

// Grant is a signed set of operations and optional scope restrictions.
type Grant struct {
	Ops   []Operation          `json:"ops"`
	Scope map[string]string    `json:"scope,omitempty"`
	Expiry time.Time           `json:"expiry,omitempty"`
	Sig   []byte               `json:"sig"`
}

// NewGrant creates a grant with the given operations and signs it
// with the provided key. If key is empty the grant is created unsigned.
func NewGrant(ops []Operation, key []byte) (*Grant, error) {
	g := &Grant{Ops: ops}
	if len(key) > 0 {
		data, err := json.Marshal(g)
		if err != nil {
			return nil, err
		}
		g.Sig, err = Sign(data, key)
		if err != nil {
			return nil, err
		}
	}
	return g, nil
}

// Permits reports whether the grant includes the given operation
// (least-privilege: absent means denied).
func (g *Grant) Permits(op Operation) bool {
	for _, o := range g.Ops {
		if o == op {
			return true
		}
	}
	return false
}

// VerifyGrant verifies the grant's integrity signature using the
// provided key. Returns nil if the signature is valid or if the
// grant was created unsigned (no key).
func VerifyGrant(g *Grant, key []byte) error {
	if len(g.Sig) == 0 {
		return nil // unsigned grant — skip verification
	}

	sig := g.Sig
	g.Sig = nil
	defer func() { g.Sig = sig }()

	data, err := json.Marshal(g)
	if err != nil {
		return err
	}

	if !Verify(sig, data, key) {
		return errors.New("grant signature verification failed")
	}
	return nil
}
