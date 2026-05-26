package cache

import (
	"sync"
)

// CatalogCache holds the compressed catalog bytes and its R2 ETag
type CatalogCache struct {
	mu    sync.RWMutex
	etag  string
	bytes []byte
}

// NewCatalogCache creates an empty cache
func NewCatalogCache() *CatalogCache {
	return &CatalogCache{}
}

// Get safely returns the current cached ETag and compressed bytes
func (c *CatalogCache) Get() (string, []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.etag, c.bytes
}

// Update safely replaces the cache with new data from R2
func (c *CatalogCache) Update(etag string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.etag = etag
	c.bytes = data
}
