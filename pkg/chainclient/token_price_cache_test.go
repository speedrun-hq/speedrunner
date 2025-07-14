package chainclient

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenPriceCache tests the TokenPriceCache functionality
func TestTokenPriceCache(t *testing.T) {
	t.Run("NewTokenPriceCache", func(t *testing.T) {
		ttl := 60 * time.Second
		cache := NewTokenPriceCache(ttl)

		require.NotNil(t, cache)
		assert.Equal(t, ttl, cache.cacheTTL)
		assert.NotNil(t, cache.cache)
	})

	t.Run("Set and Get", func(t *testing.T) {
		cache := NewTokenPriceCache(1 * time.Second)

		// Set a price
		cache.Set("ethereum", 3000.0)

		// Get the price immediately
		price, found := cache.Get("ethereum")
		assert.True(t, found)
		assert.Equal(t, 3000.0, price)

		// Get non-existent price
		_, found = cache.Get("nonexistent")
		assert.False(t, found)
	})

	t.Run("TTL expiration", func(t *testing.T) {
		cache := NewTokenPriceCache(10 * time.Millisecond)

		// Set a price
		cache.Set("ethereum", 3000.0)

		// Get immediately - should work
		price, found := cache.Get("ethereum")
		assert.True(t, found)
		assert.Equal(t, 3000.0, price)

		// Wait for TTL to expire
		time.Sleep(20 * time.Millisecond)

		// Get after expiration - should not work
		_, found = cache.Get("ethereum")
		assert.False(t, found)
	})

	t.Run("Clear", func(t *testing.T) {
		cache := NewTokenPriceCache(1 * time.Second)

		// Set multiple prices
		cache.Set("ethereum", 3000.0)
		cache.Set("matic-network", 1.0)

		// Verify they exist
		_, found := cache.Get("ethereum")
		assert.True(t, found)
		_, found = cache.Get("matic-network")
		assert.True(t, found)

		// Clear cache
		cache.Clear()

		// Verify they're gone
		_, found = cache.Get("ethereum")
		assert.False(t, found)
		_, found = cache.Get("matic-network")
		assert.False(t, found)
	})

	t.Run("Concurrent access", func(t *testing.T) {
		cache := NewTokenPriceCache(1 * time.Second)
		done := make(chan bool, 10)

		// Start multiple goroutines reading and writing
		for i := 0; i < 5; i++ {
			go func(id int) {
				tokenID := fmt.Sprintf("token-%d", id)
				cache.Set(tokenID, float64(id*1000))
				time.Sleep(1 * time.Millisecond)
				_, _ = cache.Get(tokenID)
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 5; i++ {
			<-done
		}

		// Verify all values are set
		for i := 0; i < 5; i++ {
			tokenID := fmt.Sprintf("token-%d", i)
			price, found := cache.Get(tokenID)
			assert.True(t, found)
			assert.Equal(t, float64(i*1000), price)
		}
	})
}

// TestGlobalCacheFunctions tests the global cache utility functions
func TestGlobalCacheFunctions(t *testing.T) {
	t.Run("SetGlobalCacheTTL", func(t *testing.T) {
		// Clear any existing cache
		ClearGlobalCache()

		// Set new TTL
		newTTL := 45 * time.Second
		SetGlobalCacheTTL(newTTL)

		// Verify TTL was set
		count, ttl := GetGlobalCacheStats()
		assert.Equal(t, 0, count) // Should be empty
		assert.Equal(t, newTTL, ttl)
	})

	t.Run("ClearGlobalCache", func(t *testing.T) {
		// Set some data in cache
		cache := getOrCreateCache()
		cache.Set("ethereum", 3000.0)
		cache.Set("matic-network", 1.0)

		// Verify data exists
		count, _ := GetGlobalCacheStats()
		assert.Equal(t, 2, count)

		// Clear cache
		ClearGlobalCache()

		// Verify cache is empty
		count, _ = GetGlobalCacheStats()
		assert.Equal(t, 0, count)
	})

	t.Run("GetGlobalCacheStats", func(t *testing.T) {
		// Clear cache first
		ClearGlobalCache()

		// Set TTL and add some data
		SetGlobalCacheTTL(30 * time.Second)
		cache := getOrCreateCache()
		cache.Set("ethereum", 3000.0)
		cache.Set("matic-network", 1.0)
		cache.Set("binancecoin", 500.0)

		// Get stats
		count, ttl := GetGlobalCacheStats()
		assert.Equal(t, 3, count)
		assert.Equal(t, 30*time.Second, ttl)
	})
}

// TestCacheIntegration tests the integration between cache and getTokenPriceUSD
func TestCacheIntegration(t *testing.T) {
	t.Run("Cache hit for same token", func(t *testing.T) {
		// Clear cache first
		ClearGlobalCache()

		// Set a short TTL for testing
		SetGlobalCacheTTL(1 * time.Second)

		// This test would require mocking the HTTP client
		// For now, we just verify the cache structure works
		cache := getOrCreateCache()

		// Manually set a cached price
		cache.Set("ethereum", 3000.0)

		// Verify it's cached
		price, found := cache.Get("ethereum")
		assert.True(t, found)
		assert.Equal(t, 3000.0, price)

		// Verify cache stats
		count, _ := GetGlobalCacheStats()
		assert.Equal(t, 1, count)
	})

	t.Run("Cache miss for different tokens", func(t *testing.T) {
		// Clear cache first
		ClearGlobalCache()

		cache := getOrCreateCache()

		// Set different token prices
		cache.Set("ethereum", 3000.0)
		cache.Set("matic-network", 1.0)

		// Verify they're separate
		ethPrice, found := cache.Get("ethereum")
		assert.True(t, found)
		assert.Equal(t, 3000.0, ethPrice)

		maticPrice, found := cache.Get("matic-network")
		assert.True(t, found)
		assert.Equal(t, 1.0, maticPrice)

		// Verify cache stats
		count, _ := GetGlobalCacheStats()
		assert.Equal(t, 2, count)
	})
}

// BenchmarkTokenPriceCache benchmarks the cache operations
func BenchmarkTokenPriceCache(b *testing.B) {
	cache := NewTokenPriceCache(1 * time.Second)

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tokenID := fmt.Sprintf("token-%d", i%100)
			cache.Set(tokenID, float64(i))
		}
	})

	b.Run("Get", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < 100; i++ {
			tokenID := fmt.Sprintf("token-%d", i)
			cache.Set(tokenID, float64(i))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tokenID := fmt.Sprintf("token-%d", i%100)
			_, _ = cache.Get(tokenID)
		}
	})

	b.Run("GetMiss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tokenID := fmt.Sprintf("miss-%d", i)
			_, _ = cache.Get(tokenID)
		}
	})
}
