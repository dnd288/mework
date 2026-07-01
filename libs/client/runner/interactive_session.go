package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	"mework/libs/shared/ports"
)

// Caller identifies who is driving a session: the account, its tenant, and the
// permission grant it presents. Every session operation is gated on the
// caller's account matching the session owner, the caller's tenant matching the
// session tenant, and the grant permitting the operation.
type Caller struct {
	Account core.AccountID
	Tenant  core.TenantID
	Grant   *grant.Grant
}

// SessionDeps carries the injectable dependencies for an interactive session so
// the resolver, the engine→manager mapping, the bus, and the session manager
// can be faked in tests.
type SessionDeps struct {
	// Resolver maps a definition reference to its bundle metadata.
	Resolver DefinitionResolver
	// ManagerFor builds the sandbox manager for a named engine. When nil, the
	// default runtime.NewManagerFor (local-by-default) engine dispatch is used.
	ManagerFor func(engine string) *runtime.Manager
	// Broker is the message bus used to stream turn events.
	Broker bus.Broker
	// Sessions owns the create/attach/close lifecycle and idle reaping.
	Sessions *session.Manager
	// GrantKey verifies grant signatures; nil accepts unsigned grants.
	GrantKey []byte
	// Workspace, when set, binds the session's sandbox to a working directory.
	// The zero value leaves the run unbound (SandboxID-derived workdir).
	Workspace core.Workspace

	// SessionID, when non-empty, overrides the session id used for event
	// publication.  The hub creates a session with its own id before dispatching
	// to the runner; the daemon's local session.Manager.Create generates a
	// different id, so the EventPublisher would otherwise publish to the wrong
	// control topic.  Set this to the hub's session id so events reach the
	// client that called session attach.
	SessionID core.SessionID
}

// Session drives a long-lived sandbox for one interactive, multi-turn chat. The
// sandbox is started once at OpenSession and kept alive across turns; each turn
// is fed over stdin (never argv), an in-flight turn can be cancelled without
// destroying the sandbox, and Close (or idle reaping) destroys it. Lifecycle is
// bound to the session manager and every operation is owner+tenant+grant gated.
type Session struct {
	info    core.SessionInfo
	deps    SessionDeps
	mgr     *runtime.Manager
	backend string
	sandbox ports.Sandbox
	pub     *EventPublisher

	mu        sync.Mutex
	cancelCur context.CancelFunc // cancels the in-flight turn, if any
	destroyed bool
}

// OpenSession resolves the definition, starts its sandbox exactly once, and
// registers a session owned by the caller. The caller must present a grant that
// permits spawning; otherwise the session is denied before any sandbox starts.
func OpenSession(ctx context.Context, ref string, caller Caller, deps SessionDeps) (*Session, error) {
	if err := authorize(caller, grant.OpSpawn, deps.GrantKey); err != nil {
		return nil, err
	}

	def, err := deps.Resolver.ResolveDefinition(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("definition %q not found: %w", ref, err)
	}

	managerFor := deps.ManagerFor
	if managerFor == nil {
		managerFor = runtime.NewManagerFor
	}
	engine := def.Engine
	if engine == "" {
		engine = "local"
	}
	mgr := managerFor(engine)

	info, err := deps.Sessions.Create(ctx, def.Name, def.Version, "", caller.Account, caller.Tenant)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	spec := core.RunSpec{
		AgentID:     def.Name,
		BackendName: def.Backend,
		SandboxID:   strings.ReplaceAll(ref, "@", "-") + "-" + string(info.ID),
		Workspace:   deps.Workspace,
	}
	if def.UsesImage() {
		spec.Image = def.Image
	}

	sb, err := mgr.Start(ctx, spec)
	if err != nil {
		_ = deps.Sessions.Close(ctx, info.ID)
		return nil, fmt.Errorf("start sandbox: %w", err)
	}

	// Use the caller-supplied session id for events so that the hub's
	// session control topic matches what the attach client subscribed to.
	eventSessionID := info.ID
	if deps.SessionID != "" {
		eventSessionID = deps.SessionID
	}
	return &Session{
		info:    info,
		deps:    deps,
		mgr:     mgr,
		backend: def.Backend,
		sandbox: sb,
		pub:     NewEventPublisher(deps.Broker, eventSessionID),
	}, nil
}

// Send feeds one chat turn to the long-lived sandbox over stdin and publishes
// the turn's events to the session topic. The sandbox is reused across turns.
func (s *Session) Send(ctx context.Context, caller Caller, content string) error {
	if err := s.authorizeCaller(caller, grant.OpSpawn); err != nil {
		return err
	}

	turnCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancelCur = cancel
	s.mu.Unlock()

	// stdin-not-argv: the backend name forms the command; the turn content is fed
	// over stdin so attacker-controllable content never reaches the command line.
	var out strings.Builder
	exitCode, execErr := s.sandbox.Exec(turnCtx, []string{s.backend}, strings.NewReader(content), &out, &out)

	s.mu.Lock()
	s.cancelCur = nil
	s.mu.Unlock()
	cancel()

	turn := TurnResult{Output: out.String(), Failed: execErr != nil && exitCode != 0}
	if pubErr := s.pub.PublishTurn(ctx, turn); pubErr != nil {
		return pubErr
	}
	return nil
}

// Cancel interrupts the in-flight turn, if any, by signalling the sandbox
// process and cancelling the turn's context. The sandbox is kept alive so the
// session remains usable for subsequent turns.
func (s *Session) Cancel(ctx context.Context, caller Caller) error {
	if err := s.authorizeCaller(caller, grant.OpSpawn); err != nil {
		return err
	}

	if err := s.sandbox.Signals(ctx, "interrupt"); err != nil {
		return fmt.Errorf("interrupt turn: %w", err)
	}

	s.mu.Lock()
	cancel := s.cancelCur
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Close ends the session and destroys the long-lived sandbox.
func (s *Session) Close(ctx context.Context, caller Caller) error {
	if err := s.authorizeCaller(caller, grant.OpSpawn); err != nil {
		return err
	}
	_ = s.deps.Sessions.Close(ctx, s.info.ID)
	s.destroySandbox(ctx)
	return nil
}

// Status returns the current status of the session. When the session has been
// closed (explicitly or by the idle reaper) the long-lived sandbox is destroyed
// so the sandbox's lifetime is bounded by its session.
func (s *Session) Status(ctx context.Context, caller Caller) (core.SessionStatus, error) {
	if err := s.authorizeCaller(caller, grant.OpSpawn); err != nil {
		return "", err
	}
	info, err := s.deps.Sessions.Get(ctx, s.info.ID)
	if err != nil {
		return "", err
	}
	if info.Status == core.SessionClosed {
		s.destroySandbox(ctx)
	}
	return info.Status, nil
}

// List returns the sessions belonging to the caller's own tenant. It is
// authorized (grant-verified) and tenant-scoped: the tenant is taken from the
// authenticated caller — never a caller-supplied argument — so a caller can only
// ever see its own tenant's sessions.
func (s *Session) List(ctx context.Context, caller Caller) ([]core.SessionInfo, error) {
	if err := authorize(caller, grant.OpSpawn, s.deps.GrantKey); err != nil {
		return nil, err
	}
	return s.deps.Sessions.List(ctx, caller.Tenant)
}

// destroySandbox tears down the long-lived sandbox exactly once.
func (s *Session) destroySandbox(ctx context.Context) {
	s.mu.Lock()
	if s.destroyed {
		s.mu.Unlock()
		return
	}
	s.destroyed = true
	s.mu.Unlock()
	_ = s.mgr.Destroy(ctx, s.sandbox.ID())
}

// authorizeCaller enforces owner + tenant + grant for an operation on this
// session.
func (s *Session) authorizeCaller(caller Caller, op grant.Operation) error {
	if caller.Account != s.info.Owner {
		return fmt.Errorf("session %q is owned by %q, caller is %q", s.info.ID, s.info.Owner, caller.Account)
	}
	if caller.Tenant != s.info.Tenant {
		return fmt.Errorf("session %q belongs to tenant %q, caller is in %q", s.info.ID, s.info.Tenant, caller.Tenant)
	}
	return authorize(caller, op, s.deps.GrantKey)
}

// authorize verifies the caller's grant and checks it permits the operation.
func authorize(caller Caller, op grant.Operation, key []byte) error {
	if caller.Grant == nil {
		return fmt.Errorf("no grant presented")
	}
	if err := grant.VerifyGrant(caller.Grant, key); err != nil {
		return fmt.Errorf("verify grant: %w", err)
	}
	return enforceGrant(caller.Grant, op)
}
