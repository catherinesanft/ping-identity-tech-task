package cache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// TTLCache is a generic, thread-safe cache where each entry expires
// independently based on when it was set.
type TTLCache[V any] struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]entry[V]
}

// New creates a TTLCache whose entries expire ttl after being Set.
func New[V any](ttl time.Duration) *TTLCache[V] {
	return &TTLCache[V]{
		ttl: ttl,
		m:   make(map[string]entry[V]),
	}
}

// Get returns the value for key and whether it was found and not expired.
func (c *TTLCache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(e.expiresAt) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores value under key, expiring it after the cache's configured TTL.
func (c *TTLCache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = entry[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}
