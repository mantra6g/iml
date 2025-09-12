package cache

import (
	"builder/pkg/cache/dto"
	"sync"
)

type Cache[K comparable, V dto.Versionable] struct {
	data map[K]V
	mu   sync.RWMutex
}

// NewCache creates a new Cache instance
func NewCache[K comparable, V dto.Versionable]() *Cache[K, V] {
	return &Cache[K, V]{
		data: make(map[K]V),
	}
}

// Set inserts or updates a value in the cache
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Get retrieves a value from the cache by key (returns a copy)
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

// Delete removes a key from the cache
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}
