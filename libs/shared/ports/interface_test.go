package ports_test

import (
	"reflect"
	"testing"
	"mework/libs/shared/ports"
)

func TestInterfaces_AreDefined(t *testing.T) {
	tests := []struct {
		name  string
		iface interface{}
	}{
		{"SandboxDriver", (*ports.SandboxDriver)(nil)},
		{"Sandbox", (*ports.Sandbox)(nil)},
		{"ObjectStore", (*ports.ObjectStore)(nil)},
		{"AgentBackend", (*ports.AgentBackend)(nil)},
		{"Broker", (*ports.Broker)(nil)},
		{"ProviderAdapter", (*ports.ProviderAdapter)(nil)},
		{"Notifier", (*ports.Notifier)(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := reflect.TypeOf(tt.iface)
			if typ == nil {
				t.Fatal("nil type -- interface not defined")
			}
			t.Logf("interface %s exists: %s", tt.name, typ)
		})
	}
}
