package catalog

import "mework/libs/shared/grant"

// AuthorizePull checks whether an identity with the given grant is allowed
// to pull an agent version. Returns nil if allowed, or an error describing
// the denial.
func AuthorizePull(identity, grantHeader string, g *grant.Grant, agentName string) error {
	if identity == "" {
		return ErrUnauthorized
	}
	if grantHeader == "" || g == nil {
		return ErrForbidden
	}
	if !g.Permits(grant.OpPullAgent) {
		return ErrForbidden
	}
	return nil
}

// AuthorizeDispatch checks whether an identity with the given grant is
// allowed to dispatch an agent to a runner.
func AuthorizeDispatch(identity string) error {
	if identity == "" {
		return ErrUnauthorized
	}
	return nil
}

// Sentinel errors for authorization.
var (
	ErrUnauthorized = &authError{"unauthorized"}
	ErrForbidden    = &authError{"forbidden"}
)

type authError struct{ msg string }

func (e *authError) Error() string { return e.msg }
