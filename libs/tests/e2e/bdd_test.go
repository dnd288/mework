//go:build e2e

// Package e2e is the executable BDD scenario catalog for MeWork. Each scenario reads as
// GIVEN/WHEN/THEN and is SKIPPED — the agent-hub target it exercises is not implemented
// yet (openspec/changes/c0002..c0006), and the implemented baseline keeps its own real
// tests elsewhere. These exist so the BEHAVIORS and the proposed API can be reviewed and
// evaluated in real Go before anything is built. See SCENARIOS.md for the index.
//
// Everything here is in _test.go files (package e2e): no production code, nothing wired
// into a binary. `go test ./tests/e2e/...` compiles it and reports every scenario as
// SKIP; `make test` stays green. The proposed API the scenarios drive is sketched in
// api_test.go (the review surface); the harness World is below.
package e2e

import (
	"testing"
)

// Status badges — each scenario declares whether the behavior exists today or is
// planned under a specific change. Planned scenarios skip with their change id.
const (
	Implemented  = "implemented (baseline; see internal/... real tests)"
	PlannedC0003 = "pending c0003 — message-bus"
	PlannedC0004 = "pending c0004 — agent-catalog"
	PlannedC0005 = "pending c0005 — agent-runner"
	PlannedC0006 = "pending c0006 — sandbox-runtime"
	PlannedTgt   = "pending — full agent-hub target"
	// PlannedPlatform marks real-world platform capabilities (scheduling, sessions,
	// chat, streaming, quotas, audit, notifications, artifacts) not yet captured by a
	// specific openspec change — proposed here for review.
	PlannedPlatform = "pending — platform capability (proposed)"
)

// stepKind labels a BDD step for readable skip output and future execution.
type stepKind string

const (
	given stepKind = "GIVEN"
	when  stepKind = "WHEN"
	then  stepKind = "THEN"
	and   stepKind = "AND"
)

type step struct {
	kind stepKind
	desc string
	fn   func(*World)
}

// Builder accumulates a scenario's GIVEN/WHEN/THEN steps. Run() skips the test (design
// review only) while keeping the closures so the intended API usage type-checks and can
// be read. When a capability is implemented, dropping the Skip in Run() turns the whole
// catalog into live tests with no rewrite.
type Builder struct {
	t      *testing.T
	id     string
	title  string
	status string
	steps  []step
}

// Scenario starts a BDD scenario identified by its catalog id (e.g. "BUS-03"), a title,
// and a status badge.
func Scenario(t *testing.T, id, title, status string) *Builder {
	t.Helper()
	return &Builder{t: t, id: id, title: title, status: status}
}

func (b *Builder) add(k stepKind, desc string, fn func(*World)) *Builder {
	b.steps = append(b.steps, step{kind: k, desc: desc, fn: fn})
	return b
}

// Given establishes preconditions (the World).
func (b *Builder) Given(desc string, fn func(*World)) *Builder { return b.add(given, desc, fn) }

// When performs the action under test.
func (b *Builder) When(desc string, fn func(*World)) *Builder { return b.add(when, desc, fn) }

// Then asserts the expected outcome.
func (b *Builder) Then(desc string, fn func(*World)) *Builder { return b.add(then, desc, fn) }

// And continues the previous clause.
func (b *Builder) And(desc string, fn func(*World)) *Builder {
	k := and
	if len(b.steps) > 0 {
		k = b.steps[len(b.steps)-1].kind // read as a continuation of GIVEN/WHEN/THEN
	}
	return b.add(k, desc, fn)
}

// Run finalizes the scenario. For Implemented status, it builds a World and executes
// each step's closure sequentially. For planned/pending statuses, it skips.
//
// When TEST_DATABASE_URL is set, the World is backed by the real test DB (via NewWorld).
// Without a DB, a bare World is used — all its methods panic with "design-only" (or
// dereference nil pointers), which is the expected RED failure mode: the tests must fail
// until the World harness is wired.
func (b *Builder) Run() {
	b.t.Helper()
	if b.status != Implemented {
		b.t.Skipf("[%s] %s — %s", b.id, b.title, b.status)
		return
	}

	// Implemented scenarios require the full DB-backed World.
	// NewWorld skips the test when TEST_DATABASE_URL is unset.
	w := NewWorld(b.t)
	w.TB = b.t

	for _, s := range b.steps {
		s.fn(w)
	}
}
