package channel

import (
	"context"
	"log"

	"mework/libs/server/bus"
)

// FeatureChecker is the interface for checking whether channel routing is enabled.
type FeatureChecker interface {
	IsEnabled() bool
}

// Router routes incoming events from any provider to per-resource channel
// sessions. It computes a deterministic channel key from (provider, resource),
// looks up the bound session, and either delivers the event or triggers
// auto-provisioning.
type Router struct {
	registry    Registry
	broker      bus.Broker
	provisioner interface {
		Provision(ctx context.Context, providerCode, resourceID, spec string) (string, error)
	}
	feature FeatureChecker
}

// NewRouter creates a new channel Router.
func NewRouter(registry Registry, broker bus.Broker, provisioner interface {
	Provision(ctx context.Context, providerCode, resourceID, spec string) (string, error)
}, feature FeatureChecker) *Router {
	return &Router{
		registry:    registry,
		broker:      broker,
		provisioner: provisioner,
		feature:     feature,
	}
}

// ChannelKey computes a deterministic channel key from provider code and resource ID.
// Format: "providerCode:resourceID".
func (r *Router) ChannelKey(providerCode, resourceID string) string {
	return providerCode + ":" + resourceID
}

// IsEnabled returns true if the channel routing feature flag is enabled.
// Returns false if the feature flag is nil (backward compatible).
func (r *Router) IsEnabled() bool {
	if r.feature == nil {
		return false
	}
	return r.feature.IsEnabled()
}

// Route delivers an event to the channel bound to the given provider and
// resource. If no session exists, it triggers auto-provisioning.
// Returns nil on success or when the feature flag is off (caller falls
// through to the legacy path). Returns nil even on provision failure
// (logged but not propagated).
func (r *Router) Route(ctx context.Context, providerCode, resourceID, eventType string, payload []byte) error {
	// Feature flag off or nil — caller falls through to old path
	if r.feature == nil || !r.feature.IsEnabled() {
		return nil
	}

	channelKey := r.ChannelKey(providerCode, resourceID)

	// Check channel status — draining/closed channels reject new events
	status, err := r.registry.Status(ctx, channelKey)
	if err != nil {
		return err
	}
	if status == StatusDraining || status == StatusClosed {
		return nil // Silent ignore
	}

	// Lookup existing session
	sessionID, err := r.registry.Lookup(ctx, channelKey)
	if err != nil {
		return err
	}

	// No session — auto-provision
	if sessionID == "" {
		newSessionID, err := r.provisioner.Provision(ctx, providerCode, resourceID, "")
		if err != nil {
			log.Printf("Auto-provision failed for channel %s: %v", channelKey, err)
			return nil // Caller falls through to old path
		}
		sessionID = newSessionID
	}

	// Publish to the channel topic
	topic := bus.FormatChannelTopic(providerCode, resourceID, eventType)
	return r.broker.Publish(ctx, topic, bus.Message{Payload: payload})
}
