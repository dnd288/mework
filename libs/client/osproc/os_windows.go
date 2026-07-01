//go:build windows

package osproc

import (
	"os"
	"os/exec"
	"syscall"
)

// Windows process creation flags (not all are in syscall on every Go version).
const (
	detachedProcess        = 0x00000008
	createBreakawayFromJob = 0x01000000
)

// ConfigureDetached applies Windows detach settings so the daemon survives the
// parent console closing and any Job Object limits.
func ConfigureDetached(cmd *exec.Cmd, logFile *os.File) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createBreakawayFromJob,
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
}
