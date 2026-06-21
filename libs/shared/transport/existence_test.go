package transport_test

import (
	"testing"
	"mework/libs/shared/transport"
)

func TestTransportPackage_Compiles(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
	}{
		{"SSEEvent", transport.SSEEvent{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.val
		})
	}
}
