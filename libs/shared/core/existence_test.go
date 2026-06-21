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
