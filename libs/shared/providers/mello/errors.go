package mello

import "fmt"

// Exit codes mirror the CLI's error taxonomy (0 success … 5 validation).
const (
	ExitOK         = 0
	ExitGeneric    = 1
	ExitNetwork    = 2
	ExitAuth       = 3
	ExitNotFound   = 4
	ExitValidation = 5
)

// APIError is a non-2xx response from the Mello API. The wire shape is
// {error, message, fields} (see exceptions.py:raise_for_status).
type APIError struct {
	StatusCode int               `json:"-"`
	ErrorCode  string            `json:"error"`
	Message    string            `json:"message"`
	Fields     map[string]string `json:"fields,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d] %s: %s", e.StatusCode, e.ErrorCode, e.Message)
}

// Sentinel category predicates let callers branch without type lists.
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == 401 }
func (e *APIError) IsForbidden() bool    { return e.StatusCode == 403 }
func (e *APIError) IsNotFound() bool     { return e.StatusCode == 404 }

func (e *APIError) IsValidation() bool {
	return e.StatusCode == 422 || e.ErrorCode == "validation_error"
}

// IsRateLimited reports a 429 or an explicit rate_limited error code.
func (e *APIError) IsRateLimited() bool {
	return e.StatusCode == 429 || e.ErrorCode == "rate_limited"
}

// ExitCode maps an error to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		return ExitNetwork // non-API errors are transport/IO failures
	}
	switch {
	case apiErr.IsUnauthorized(), apiErr.IsForbidden():
		return ExitAuth
	case apiErr.IsNotFound():
		return ExitNotFound
	case apiErr.IsValidation():
		return ExitValidation
	default:
		return ExitGeneric
	}
}
