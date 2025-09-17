package cache

import (
	"builder/pkg/cache/dto"
	"sync"
)

type Cache[K comparable, V dto.Versionable] struct {
	data map[K]*V
	mu   sync.RWMutex
}

// NewCache creates a new Cache instance.
func NewCache[K comparable, V dto.Versionable]() *Cache[K, V] {
	return &Cache[K, V]{
		data: make(map[K]*V),
	}
}

// Set inserts or updates a value in the cache.
// Accepts by value, but stores a pointer internally.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	vCopy := value
	c.data[key] = &vCopy
}

// Get retrieves a value by key, returning a safe copy.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if ptr, ok := c.data[key]; ok {
		return *ptr, true
	}
	var zero V
	return zero, false
}
