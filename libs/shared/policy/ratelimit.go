package policy

import (
	"sync"
	"time"
)

// RateLimiter tracks per-sender message counts in sliding windows.
// Safe for concurrent use.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	window  time.Duration
}

type rateBucket struct {
	count int
	start time.Time
}

// NewRateLimiter creates a RateLimiter with 1-minute sliding windows.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*rateBucket),
		window:  time.Minute,
	}
}

// Allow returns true if senderKey has not exceeded the given limit in the
// current sliding window. If limit <= 0, all messages are allowed.
func (rl *RateLimiter) Allow(senderKey string, limit int) bool {
	if limit <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[senderKey]
	if !ok || now.Sub(b.start) > rl.window {
		b = &rateBucket{start: now, count: 0}
		rl.buckets[senderKey] = b
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}

// Cleanup removes stale buckets. Run periodically in long-running servers.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for k, b := range rl.buckets {
		if b.start.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}
