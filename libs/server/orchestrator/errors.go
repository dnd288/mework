package orchestrator

import "errors"

var (
	// ErrNoEligibleRunner is returned by Select when no runner is eligible.
	ErrNoEligibleRunner = errors.New("no eligible runner found for dispatch")

	// ErrRunnerNotFound is returned when a runner is not found in the index.
	ErrRunnerNotFound = errors.New("runner not found")
)
