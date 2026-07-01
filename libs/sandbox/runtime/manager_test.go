package runtime

import (
	"context"
	"errors"
	"io"
	"testing"

	"mework/libs/shared/core"
	"mework/libs/shared/ports"
)

// fakeSandbox is a ports.Sandbox that records Mount calls.
type fakeSandbox struct {
	id            string
	mountErr      error
	mountCalls    int
	lastWorkspace core.Workspace
	lastTarget    string
}

func (f *fakeSandbox) ID() string { return f.id }

func (f *fakeSandbox) Exec(ctx context.Context, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return 0, nil
}

func (f *fakeSandbox) Mount(ctx context.Context, workspace core.Workspace, targetPath string) error {
	f.mountCalls++
	f.lastWorkspace = workspace
	f.lastTarget = targetPath
	return f.mountErr
}

func (f *fakeSandbox) Signals(ctx context.Context, sig string) error { return nil }

// fakeDriver is a ports.SandboxDriver that returns a preconfigured fakeSandbox
// and records Destroy calls so cleanup-on-error can be asserted.
type fakeDriver struct {
	sandbox      *fakeSandbox
	startErr     error
	destroyCalls int
}

func (d *fakeDriver) Caps() core.SandboxCaps { return core.SandboxCaps{} }

func (d *fakeDriver) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
	if d.startErr != nil {
		return nil, d.startErr
	}
	return d.sandbox, nil
}

func (d *fakeDriver) Stop(ctx context.Context, sandboxID string) error { return nil }

func (d *fakeDriver) Destroy(ctx context.Context, sandboxID string) error {
	d.destroyCalls++
	return nil
}

// TestManagerStart_MountsBoundWorkspace covers the delta-spec "Workspace-bound
// session" requirement: when spec.Workspace.Path is set, Manager.Start mounts the
// workspace into the (container) sandbox via the Sandbox.Mount seam; when unset,
// Mount is never called (the unbound path is unchanged); and a Mount error
// propagates and the just-started sandbox is cleaned up rather than registered.
func TestManagerStart_MountsBoundWorkspace(t *testing.T) {
	tests := []struct {
		name           string
		workspace      core.Workspace
		mountErr       error
		wantMountCalls int
		wantErr        bool
		wantRunning    int
	}{
		{
			name:           "workspace bound -> Mount called",
			workspace:      core.Workspace{ID: "ws-1", Path: "/tmp/work/ws-1"},
			wantMountCalls: 1,
			wantErr:        false,
			wantRunning:    1,
		},
		{
			name:           "no workspace -> Mount skipped",
			workspace:      core.Workspace{},
			wantMountCalls: 0,
			wantErr:        false,
			wantRunning:    1,
		},
		{
			name:           "Mount error propagates",
			workspace:      core.Workspace{ID: "ws-2", Path: "/tmp/work/ws-2"},
			mountErr:       errors.New("mount boom"),
			wantMountCalls: 1,
			wantErr:        true,
			wantRunning:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := &fakeSandbox{id: "sb-" + tt.name, mountErr: tt.mountErr}
			drv := &fakeDriver{sandbox: sb}
			m := NewManager(drv)

			spec := core.RunSpec{SandboxID: sb.id, Workspace: tt.workspace}

			_, err := m.Start(context.Background(), spec)

			if tt.wantErr && err == nil {
				t.Fatalf("Start() error = nil, want non-nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Start() unexpected error: %v", err)
			}
			if tt.wantErr && tt.mountErr != nil && !errors.Is(err, tt.mountErr) {
				t.Errorf("Start() error = %v, want it to wrap %v", err, tt.mountErr)
			}

			if sb.mountCalls != tt.wantMountCalls {
				t.Errorf("Mount call count = %d, want %d", sb.mountCalls, tt.wantMountCalls)
			}

			if tt.wantMountCalls > 0 {
				if sb.lastWorkspace != tt.workspace {
					t.Errorf("Mount workspace = %+v, want %+v", sb.lastWorkspace, tt.workspace)
				}
				if sb.lastTarget == "" {
					t.Errorf("Mount target path = %q, want non-empty", sb.lastTarget)
				}
			}

			if got := m.ActiveCount(); got != tt.wantRunning {
				t.Errorf("ActiveCount() = %d, want %d", got, tt.wantRunning)
			}

			if tt.wantErr {
				if drv.destroyCalls == 0 {
					t.Errorf("expected the just-started sandbox to be destroyed on Mount error, but Destroy was not called")
				}
			}
		})
	}
}
