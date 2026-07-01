package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"mework/libs/sandbox"
	"mework/libs/sandbox/runtime"
	"mework/libs/shared/core"
)

// ErrDefinitionNotFound is returned by a DefinitionResolver when a reference
// resolves to no published definition. RunByReference surfaces it as a
// not-found result before any sandbox is started.
var ErrDefinitionNotFound = errors.New("definition not found")

// DefinitionResolver resolves a prebuilt definition reference (e.g.
// "local-claude@1.0.0" or a moving "latest" pointer) to its bundle metadata.
type DefinitionResolver interface {
	ResolveDefinition(ctx context.Context, ref string) (*sandbox.SandboxBundleMetadata, error)
}

// RunDeps carries the injectable dependencies for RunByReference so the
// resolver and the engine→manager mapping can be faked in tests.
type RunDeps struct {
	// Resolver maps a reference to a definition.
	Resolver DefinitionResolver
	// ManagerFor builds the sandbox manager for a named engine. When nil, the
	// default runtime.NewManagerFor (local-by-default) engine dispatch is used.
	ManagerFor func(engine string) *runtime.Manager
	// SandboxID overrides the sandbox ID; when empty it is derived from the ref.
	SandboxID string
	// Workspace, when set, binds the run's sandbox to a working directory. The
	// zero value leaves the run unbound (SandboxID-derived workdir).
	Workspace core.Workspace
}

// RunByReference runs a prebuilt definition by reference as a one-shot: it
// resolves the definition, maps its engine through ManagerFor (default local),
// starts the sandbox, and runs the agent backend feeding the instruction over
// stdin — never argv. An unresolved reference yields a not-found result before
// any sandbox is started. Container engines run from the definition's pinned
// image; the local engine ignores it. The manager's running map enforces
// one-agent-per-sandbox, surfaced here as the duplicate-sandbox error.
func RunByReference(ctx context.Context, ref, instruction string, deps RunDeps) core.Result {
	def, err := deps.Resolver.ResolveDefinition(ctx, ref)
	if err != nil {
		return core.Result{Error: fmt.Sprintf("definition %q not found: %v", ref, err)}
	}

	managerFor := deps.ManagerFor
	if managerFor == nil {
		managerFor = runtime.NewManagerFor
	}
	// An omitted engine defaults to local.
	engine := def.Engine
	if engine == "" {
		engine = "local"
	}
	mgr := managerFor(engine)

	sandboxID := deps.SandboxID
	if sandboxID == "" {
		sandboxID = strings.ReplaceAll(ref, "@", "-")
	}

	spec := core.RunSpec{
		AgentID:     def.Name,
		BackendName: def.Backend,
		Task:        instruction,
		SandboxID:   sandboxID,
		Workspace:   deps.Workspace,
	}
	// Container engines materialize from the pinned image; local ignores it.
	if def.UsesImage() {
		spec.Image = def.Image
	}

	s, err := mgr.Start(ctx, spec)
	if err != nil {
		return core.Result{Error: fmt.Sprintf("start sandbox: %v", err)}
	}

	// stdin-not-argv: the backend name forms the command; the instruction is fed
	// over stdin so attacker-controllable content never reaches the command line.
	var stdout, stderr bytes.Buffer
	exitCode, execErr := s.Exec(ctx, []string{def.Backend}, strings.NewReader(instruction), &stdout, &stderr)

	res := core.Result{
		Output:   stdout.String() + stderr.String(),
		ExitCode: exitCode,
	}
	if execErr != nil {
		if exitCode <= 0 {
			res.ExitCode = -1
		}
		res.Error = execErr.Error()
	}
	return res
}
