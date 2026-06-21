// Package plugin provides a generic driver/engine registry. Backends
// register themselves at init time, and consumers look them up by name.
// This lets binaries select which drivers to link without changing
// consumer code.
package plugin

import "sync"

var (
	mu      sync.RWMutex
	drivers = map[string]any{}
)

// Register registers a driver under the given name. If a driver with the
// same name already exists it is overwritten.
func Register(name string, driver any) {
	mu.Lock()
	defer mu.Unlock()
	drivers[name] = driver
}

// Open returns the driver registered under name, or false if none exists.
func Open(name string) (any, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := drivers[name]
	return d, ok
}
