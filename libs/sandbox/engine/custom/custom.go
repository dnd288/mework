// Package custom provides a way to plug in a custom sandbox engine from an
// external Go module. It implements the ports.SandboxDriver contract and is
// wired by blank-importing the implementation into the shared/plugin registry.
package custom

import (
	"context"
	"fmt"
	"io"

	"mework/libs/shared/core"
	"mework/libs/shared/plugin"
	"mework/libs/shared/ports"
)

// DriverName is the name used in config to select the custom driver.
const DriverName = "custom"

// Driver wraps a dynamically-loaded sandbox engine from the plugin registry.
type Driver struct {
	inner ports.SandboxDriver
}

// New creates a custom Driver by looking up the registered engine in the
// plugin registry. Returns an error if no engine is registered.
func New() (*Driver, error) {
	raw, ok := plugin.Open(DriverName)
	if !ok {
		return nil, fmt.Errorf("no custom sandbox engine registered under %q", DriverName)
	}
	inner, ok := raw.(ports.SandboxDriver)
	if !ok {
		return nil, fmt.Errorf("registered plugin %q does not implement ports.SandboxDriver", DriverName)
	}
	return &Driver{inner: inner}, nil
}

// Caps delegates to the inner engine.
func (d *Driver) Caps() core.SandboxCaps {
	return d.inner.Caps()
}

// Start delegates to the inner engine.
func (d *Driver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	return d.inner.Start(ctx, spec)
}

// Stop delegates to the inner engine.
func (d *Driver) Stop(ctx context.Context, sandboxID string) error {
	return d.inner.Stop(ctx, sandboxID)
}

// Destroy delegates to the inner engine.
func (d *Driver) Destroy(ctx context.Context, sandboxID string) error {
	return d.inner.Destroy(ctx, sandboxID)
}

// customSandbox wraps a sandbox returned by a custom engine.
type customSandbox struct {
	inner    ports.Sandbox
	innerID  string
}

func (s *customSandbox) ID() string { return s.innerID }

func (s *customSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return s.inner.Exec(ctx, command, stdin, stdout, stderr)
}

func (s *customSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	return s.inner.Mount(ctx, workspace, targetPath)
}

func (s *customSandbox) Signals(ctx context.Context, sig string) error {
	return s.inner.Signals(ctx, sig)
}
