package core_test

import (
	"testing"
	"mework/libs/shared/core"
)

func TestCoreTypes_CompileAndExist(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
	}{
		{"Agent", core.Agent{}},
		{"Run", core.Run{}},
		{"Session", core.Session{}},
		{"Grant", core.Grant{}},
		{"Topic", core.Topic{}},
		{"Message", core.Message{}},
		{"RunSpec", core.RunSpec{}},
		{"Result", core.Result{}},
		{"Workspace", core.Workspace{}},
		{"ObjectRef", core.ObjectRef{}},
		{"ObjectInfo", core.ObjectInfo{}},
		{"Hook", core.Hook{}},
		{"SandboxCaps", core.SandboxCaps{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.val
		})
	}
}

// TestRunSpec_WorkspaceField asserts the additive RunSpec.Workspace field:
// the zero value of RunSpec has an empty Workspace (no regression for the
// c0026 unbound paths), and a RunSpec built with a Workspace round-trips its
// {ID, Path}. Realises delta-spec scenario "Unbound run is unchanged".
func TestRunSpec_WorkspaceField(t *testing.T) {
	tests := []struct {
		name      string
		spec      core.RunSpec
		wantWS    core.Workspace
		wantEmpty bool
	}{
		{
			name:      "zero value has an empty workspace",
			spec:      core.RunSpec{},
			wantWS:    core.Workspace{},
			wantEmpty: true,
		},
		{
			name:   "workspace round-trips ID and Path",
			spec:   core.RunSpec{Workspace: core.Workspace{ID: "ws-1", Path: "/tmp/work/ws-1"}},
			wantWS: core.Workspace{ID: "ws-1", Path: "/tmp/work/ws-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantEmpty {
				if (tt.spec.Workspace != core.Workspace{}) {
					t.Errorf("zero RunSpec.Workspace = %+v, want empty", tt.spec.Workspace)
				}
			}
			if tt.spec.Workspace != tt.wantWS {
				t.Errorf("RunSpec.Workspace = %+v, want %+v", tt.spec.Workspace, tt.wantWS)
			}
		})
	}
}
