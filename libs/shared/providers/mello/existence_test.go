package mello_test

import (
	"testing"
	"mework/libs/shared/providers/mello"
)

func TestMelloPackage_Compiles(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
	}{
		{"Client", mello.Client{}},
		{"Ticket", mello.Ticket{}},
		{"Board", mello.Board{}},
		{"APIError", mello.APIError{}},
		{"NewClient", mello.NewClient("", "", 0, "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.val
		})
	}
}
