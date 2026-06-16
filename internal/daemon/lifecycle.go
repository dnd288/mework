// Package daemon implements the mework agent-runtime daemon: process
// lifecycle (pid/log/health) plus the server poll-based consumer loop.
package daemon

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"syscall"

	"mework/internal/cli"
)

// healthBasePort is the base for the per-profile local health/shutdown port.
const healthBasePort = 19514

// HealthPort returns a deterministic loopback port for a profile so `stop`
// can reach a running daemon without storing the port separately.
func HealthPort(profile string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(profile))
	// Keep the offset small so ports stay in a predictable range.
	return healthBasePort + int(h.Sum32()%1000)
}

// WritePID records the current process id in the profile pid file.
func WritePID(profile string) error {
	if err := os.MkdirAll(cli.ProfileDir(profile), 0o700); err != nil {
		return err
	}
	pid := strconv.Itoa(os.Getpid())
	return os.WriteFile(cli.PidPath(profile), []byte(pid), 0o600)
}

// ReadPID returns the recorded pid, or 0 if no pid file exists.
func ReadPID(profile string) (int, error) {
	data, err := os.ReadFile(cli.PidPath(profile))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("corrupt pid file: %w", err)
	}
	return pid, nil
}

// RemovePID deletes the pid file (ignoring a missing file).
func RemovePID(profile string) error {
	err := os.Remove(cli.PidPath(profile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsRunning reports whether a process with the recorded pid is alive. It checks
// liveness via signal 0 rather than trusting the pid file's mere existence, so
// a stale file after a crash is not mistaken for a running daemon.
func IsRunning(profile string) (bool, int) {
	pid, err := ReadPID(profile)
	if err != nil || pid <= 0 {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false, 0
	}
	return true, pid
}

// OpenLogFile opens (creating/appending) the profile daemon log.
func OpenLogFile(profile string) (*os.File, error) {
	if err := os.MkdirAll(cli.ProfileDir(profile), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(cli.LogPath(profile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
