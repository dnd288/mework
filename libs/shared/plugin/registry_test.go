package plugin_test

import (
	"testing"
	"mework/libs/shared/plugin"
)

func TestRegister_Open_RoundTrip(t *testing.T) {
	stub := "test-stub"
	plugin.Register("test-driver", stub)

	got, ok := plugin.Open("test-driver")
	if !ok {
		t.Fatal("Open returned ok=false for registered driver")
	}
	if v, ok := got.(string); !ok || v != stub {
		t.Fatalf("Open returned %v (type %T), want %q", got, got, stub)
	}

	_, ok = plugin.Open("nonexistent")
	if ok {
		t.Error("Open returned ok=true for unregistered name")
	}
}

func TestRegister_Overwrite(t *testing.T) {
	plugin.Register("overwrite-test", "first")
	plugin.Register("overwrite-test", "second")
	got, ok := plugin.Open("overwrite-test")
	if !ok {
		t.Fatal("Open returned ok=false after overwrite")
	}
	if v, ok := got.(string); !ok || v != "second" {
		t.Fatalf("expected \"second\", got %v", got)
	}
}
