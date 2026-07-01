// Package log defines a minimal logging interface shared across all mework
// components. Each component provides its own concrete implementation.
package log

// Logger is the mework-wide logging interface. Every component logs through
// this interface so the binary can wire its own logger.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// noopLogger is a default no-op implementation used when no logger is wired.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// NewNoopLogger returns a no-op logger that discards all log entries.
func NewNoopLogger() Logger { return noopLogger{} }
