package channel

import "sync/atomic"

// FeatureFlag controls whether channel routing is enabled.
// When disabled, the webhook handler falls through to the legacy
// runner.<profile>.dispatch publish path.
type FeatureFlag struct {
	enabled atomic.Bool
}

// NewFeatureFlag creates a FeatureFlag with the given initial state.
func NewFeatureFlag(enabled bool) *FeatureFlag {
	f := &FeatureFlag{}
	f.enabled.Store(enabled)
	return f
}

// IsEnabled reports whether channel routing is active.
func (f *FeatureFlag) IsEnabled() bool {
	return f.enabled.Load()
}

// SetEnabled sets the enabled state of the feature flag.
func (f *FeatureFlag) SetEnabled(v bool) {
	f.enabled.Store(v)
}
