package runner

import (
	"context"

	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus"
	"mework/libs/server/session"
	"mework/libs/shared/core"
)

// StartOptions carries everything needed to open a workspace-bound interactive
// session. Both start modes — server and local-direct — funnel through this one
// surface; they differ only in the Resolver (HTTPDefinitionResolver vs.
// FileDefinitionResolver) and identity (a server-issued grant vs. a
// locally-minted OpSpawn grant + GrantKey). There is no other behavioral
// difference: in both modes the agent runs as a sandbox on the client and the
// session opens on the runner (no server endpoint).
type StartOptions struct {
	// Ref is the definition reference to resolve (e.g. "name@version").
	Ref string
	// Resolver maps Ref to bundle metadata. Server mode supplies an
	// HTTPDefinitionResolver; local-direct supplies a FileDefinitionResolver
	// reading mework.yml from WorkspaceDir (contacting no server).
	Resolver DefinitionResolver
	// WorkspaceDir is the directory the session's sandbox is bound to. It
	// becomes RunSpec.Workspace.Path so produced artifacts persist on disk and
	// are readable back via workspacefs.NewLocal.
	WorkspaceDir string
	// Caller identifies the account/tenant and presents the grant. The grant
	// must permit OpSpawn or the start is rejected before any sandbox starts.
	Caller Caller
	// GrantKey verifies the caller's grant signature; nil accepts unsigned
	// grants. Local-direct mints its grant with a local key and passes it here.
	GrantKey []byte
	// ManagerFor builds the sandbox manager for a named engine; nil uses the
	// default local-by-default engine dispatch.
	ManagerFor func(engine string) *runtime.Manager
	// Broker streams turn events.
	Broker bus.Broker
	// Sessions owns the session lifecycle.
	Sessions *session.Manager
}

// StartWorkspaceSession opens an interactive session bound to opts.WorkspaceDir.
// It is a thin start surface over OpenSession: it sets SessionDeps.Workspace to
// the workspace directory and otherwise delegates resolution, grant enforcement
// (OpSpawn), and sandbox start to OpenSession. The two start modes are expressed
// purely by the Resolver and grant the caller supplies in opts.
func StartWorkspaceSession(ctx context.Context, opts StartOptions) (*Session, error) {
	return OpenSession(ctx, opts.Ref, opts.Caller, SessionDeps{
		Resolver:   opts.Resolver,
		ManagerFor: opts.ManagerFor,
		Broker:     opts.Broker,
		Sessions:   opts.Sessions,
		GrantKey:   opts.GrantKey,
		Workspace:  core.Workspace{Path: opts.WorkspaceDir},
	})
}
