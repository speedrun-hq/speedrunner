package chainclient

import (
	"sync"
	"time"
)

// TokenPriceCache manages cached token prices to avoid duplicate API calls
type TokenPriceCache struct {
	mu       sync.RWMutex
	cache    map[string]*cachedPrice
	cacheTTL time.Duration
}

// cachedPrice represents a cached token price with timestamp
type cachedPrice struct {
	price     float64
	timestamp time.Time
}

// NewTokenPriceCache creates a new token price cache
func NewTokenPriceCache(cacheTTL time.Duration) *TokenPriceCache {
	return &TokenPriceCache{
		cache:    make(map[string]*cachedPrice),
		cacheTTL: cacheTTL,
	}
}

// Get retrieves a cached price if it's still valid, otherwise returns nil
func (c *TokenPriceCache) Get(tokenID string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.cache[tokenID]
	if !exists {
		return 0, false
	}

	// Check if cache is still valid
	if time.Since(cached.timestamp) > c.cacheTTL {
		return 0, false
	}

	return cached.price, true
}

// Set stores a price in the cache with current timestamp
func (c *TokenPriceCache) Set(tokenID string, price float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[tokenID] = &cachedPrice{
		price:     price,
		timestamp: time.Now(),
	}
}

// Clear removes all cached entries
func (c *TokenPriceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*cachedPrice)
}

// globalTokenPriceCache is a shared cache instance
var globalTokenPriceCache = NewTokenPriceCache(5 * time.Minute)
var globalCacheMu sync.Mutex

// getOrCreateCache returns the global cache instance
func getOrCreateCache() *TokenPriceCache {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	if globalTokenPriceCache == nil {
		globalTokenPriceCache = NewTokenPriceCache(30 * time.Minute)
	}

	return globalTokenPriceCache
}

// SetGlobalCacheTTL allows changing the cache TTL for the global cache
func SetGlobalCacheTTL(ttl time.Duration) {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	globalTokenPriceCache = NewTokenPriceCache(ttl)
}

// ClearGlobalCache clears all cached token prices
func ClearGlobalCache() {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	if globalTokenPriceCache != nil {
		globalTokenPriceCache.Clear()
	}
}

// GetGlobalCacheStats returns basic statistics about the cache
func GetGlobalCacheStats() (int, time.Duration) {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	if globalTokenPriceCache == nil {
		return 0, 0
	}

	globalTokenPriceCache.mu.RLock()
	defer globalTokenPriceCache.mu.RUnlock()

	return len(globalTokenPriceCache.cache), globalTokenPriceCache.cacheTTL
}
