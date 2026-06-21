package channel

import "sync/atomic"

// Metrics holds counters and gauges for channel routing operations.
type Metrics struct {
	// ActiveChannelCount tracks the number of active channel bindings (gauge).
	ActiveChannelCount atomic.Int64

	// ProvisionErrors counts failed auto-provision attempts (counter).
	ProvisionErrors atomic.Int64
}

// GlobalMetrics is the shared metrics instance for channel operations.
var GlobalMetrics Metrics
