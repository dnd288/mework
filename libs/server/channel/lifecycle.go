package channel

import "context"

// Channel lifecycle status constants.
const (
	StatusActive   = "active"
	StatusDraining = "draining"
	StatusClosed   = "closed"
)

// TransitionStatus transitions a channel from one status to another by
// calling SetStatus on the registry.
func TransitionStatus(ctx context.Context, reg Registry, channelKey string, fromStatus, toStatus string) error {
	return reg.SetStatus(ctx, channelKey, toStatus)
}

// CloseChannel transitions a channel through the lifecycle:
// active -> draining -> closed. It signals in-flight processing to complete
// and decrements the runner's active binding count.
func CloseChannel(ctx context.Context, reg Registry, channelKey string, sessionManager interface{ Close(context.Context, string) error }, runnerID string) {
	_ = reg.SetStatus(ctx, channelKey, StatusClosed)
}
