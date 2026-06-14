//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detachAttrs returns SysProcAttr that detaches the child from the controlling
// terminal session (Setsid) so it survives the parent shell exiting.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// configureDetached applies platform detach settings to a background daemon cmd.
func configureDetached(cmd *exec.Cmd, logFile *os.File) {
	cmd.SysProcAttr = detachAttrs()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
}
