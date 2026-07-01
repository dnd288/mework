// Package agent detects locally-installed AI CLIs for the sandbox.
package agent

import (
	"os/exec"
)

// DefaultBackends lists the AI CLIs the daemon supports, in preference order.
var DefaultBackends = []string{"claude", "codex", "opencode", "windows-claude", "v0"}

// Backend is a resolved AI CLI: its name and absolute path.
type Backend struct {
	Name string
	Path string
}

// Detect resolves the first available backend from the requested list (or
// DefaultBackends when names is empty), searching PATH. Returns ok=false when
// none are installed.
func Detect(names []string) (Backend, bool) {
	if len(names) == 0 {
		names = DefaultBackends
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return Backend{Name: name, Path: path}, true
		}
	}
	return Backend{}, false
}

// DetectAll returns every available backend from the candidate list.
func DetectAll(names []string) []Backend {
	if len(names) == 0 {
		names = DefaultBackends
	}
	var found []Backend
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			found = append(found, Backend{Name: name, Path: path})
		}
	}
	return found
}
