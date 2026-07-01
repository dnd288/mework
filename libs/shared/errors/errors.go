// Package errors holds shared error types used across all mework components.
// These provide consistent error classification without coupling to any
// specific component's error handling.
package errors

// NotFound indicates that a requested resource does not exist.
type NotFound struct {
	Kind string
	ID   string
}

func (e *NotFound) Error() string {
	return e.Kind + " not found: " + e.ID
}

// AlreadyExists indicates that a resource cannot be created because it already exists.
type AlreadyExists struct {
	Kind string
	ID   string
}

func (e *AlreadyExists) Error() string {
	return e.Kind + " already exists: " + e.ID
}

// PermissionDenied indicates the caller lacks authorization.
type PermissionDenied struct {
	Action string
	Reason string
}

func (e *PermissionDenied) Error() string {
	return "permission denied: " + e.Action + " (" + e.Reason + ")"
}

// ValidationError indicates invalid input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + " " + e.Message
}
