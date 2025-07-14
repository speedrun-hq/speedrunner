package chainclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetTokenPriceUSD_Live tests the GetTokenPriceUSD function with live API calls
// To run these tests manually, comment out the t.Skip() calls
func TestGetTokenPriceUSD_Live(t *testing.T) {
	// Skip by default - uncomment to run live tests
	t.Skip("Skipping live tests by default. Uncomment this line to run live tests.")

	ctx := context.Background()

	t.Run("Ethereum Mainnet", func(t *testing.T) {
		// t.Skip("Uncomment to test Ethereum mainnet")

		price, err := getTokenPriceUSD(ctx, 1)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 10000.0) // Reasonable upper bound for ETH price

		t.Logf("Ethereum price: $%.2f", price)
	})

	t.Run("Polygon", func(t *testing.T) {
		// t.Skip("Uncomment to test Polygon")

		price, err := getTokenPriceUSD(ctx, 137)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 100.0) // Reasonable upper bound for MATIC price

		t.Logf("Polygon (MATIC) price: $%.4f", price)
	})

	t.Run("Arbitrum", func(t *testing.T) {
		// t.Skip("Uncomment to test Arbitrum")

		price, err := getTokenPriceUSD(ctx, 42161)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 10000.0) // Reasonable upper bound for ETH price

		t.Logf("Arbitrum (ETH) price: $%.2f", price)
	})

	t.Run("BSC", func(t *testing.T) {
		// t.Skip("Uncomment to test BSC")

		price, err := getTokenPriceUSD(ctx, 56)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 1000.0) // Reasonable upper bound for BNB price

		t.Logf("BSC (BNB) price: $%.2f", price)
	})

	t.Run("Avalanche", func(t *testing.T) {
		// t.Skip("Uncomment to test Avalanche")

		price, err := getTokenPriceUSD(ctx, 43114)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 1000.0) // Reasonable upper bound for AVAX price

		t.Logf("Avalanche (AVAX) price: $%.2f", price)
	})

	t.Run("Unsupported Chain ID", func(t *testing.T) {
		// t.Skip("Uncomment to test unsupported chain ID")

		_, err := getTokenPriceUSD(ctx, 999999)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported chain ID")
	})

	t.Run("Context Timeout", func(t *testing.T) {
		// t.Skip("Uncomment to test context timeout")

		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()

		_, err := getTokenPriceUSD(timeoutCtx, 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "context deadline exceeded")
	})
}

// TestGetTokenPriceUSD_Cache_Live tests the cache functionality with live API calls
func TestGetTokenPriceUSD_Cache_Live(t *testing.T) {
	// Skip by default - uncomment to run live tests
	t.Skip("Skipping cache live tests by default. Uncomment this line to run live tests.")

	ctx := context.Background()

	t.Run("Cache sharing for same token", func(t *testing.T) {
		// t.Skip("Uncomment to test cache sharing")

		// Clear cache first
		ClearGlobalCache()
		SetGlobalCacheTTL(30 * time.Second)

		// Test chains that use the same token (ETH)
		ethChains := []int{1, 42161, 8453} // Ethereum, Arbitrum, Base

		// Get prices for all ETH chains
		var prices []float64
		for _, chainID := range ethChains {
			price, err := getTokenPriceUSD(ctx, chainID)
			require.NoError(t, err)
			require.Greater(t, price, 0.0)
			prices = append(prices, price)
			t.Logf("Chain %d (ETH) price: $%.2f", chainID, price)
		}

		// All prices should be identical since they use the same token
		for i := 1; i < len(prices); i++ {
			assert.InDelta(t, prices[0], prices[i], 0.01, "ETH prices should be identical across chains")
		}

		// Verify cache stats
		count, ttl := GetGlobalCacheStats()
		assert.Equal(t, 1, count, "Should only have 1 cached token (ethereum)")
		assert.Equal(t, 30*time.Second, ttl)
	})

	t.Run("Cache separation for different tokens", func(t *testing.T) {
		// t.Skip("Uncomment to test cache separation")

		// Clear cache first
		ClearGlobalCache()
		SetGlobalCacheTTL(30 * time.Second)

		// Test chains with different tokens
		ethPrice, err := getTokenPriceUSD(ctx, 1) // Ethereum
		require.NoError(t, err)
		require.Greater(t, ethPrice, 0.0)

		maticPrice, err := getTokenPriceUSD(ctx, 137) // Polygon
		require.NoError(t, err)
		require.Greater(t, maticPrice, 0.0)

		// Prices should be different
		assert.NotEqual(t, ethPrice, maticPrice, "ETH and MATIC prices should be different")

		// Verify cache stats
		count, _ := GetGlobalCacheStats()
		assert.Equal(t, 2, count, "Should have 2 cached tokens (ethereum and matic-network)")

		t.Logf("Ethereum price: $%.2f", ethPrice)
		t.Logf("Polygon (MATIC) price: $%.4f", maticPrice)
	})

	t.Run("Cache TTL behavior", func(t *testing.T) {
		// t.Skip("Uncomment to test cache TTL")

		// Clear cache and set short TTL
		ClearGlobalCache()
		SetGlobalCacheTTL(1 * time.Second)

		// Get price for Ethereum
		price1, err := getTokenPriceUSD(ctx, 1)
		require.NoError(t, err)
		require.Greater(t, price1, 0.0)

		// Verify cache hit
		count1, _ := GetGlobalCacheStats()
		assert.Equal(t, 1, count1)

		// Wait for TTL to expire
		time.Sleep(2 * time.Second)

		// Get price again - should trigger new API call
		price2, err := getTokenPriceUSD(ctx, 1)
		require.NoError(t, err)
		require.Greater(t, price2, 0.0)

		// Prices should be similar (within 5% due to market fluctuations)
		priceDiff := abs(price1 - price2)
		priceAvg := (price1 + price2) / 2
		priceDiffPercent := (priceDiff / priceAvg) * 100

		assert.Less(t, priceDiffPercent, 5.0, "Price difference should be less than 5%%")

		t.Logf("Price 1: $%.2f", price1)
		t.Logf("Price 2: $%.2f", price2)
		t.Logf("Difference: %.2f%%", priceDiffPercent)
	})
}

// TestGetTokenPriceUSD_Concurrent_Cache tests concurrent access with cache
func TestGetTokenPriceUSD_Concurrent_Cache(t *testing.T) {
	// Skip by default - uncomment to run live tests
	t.Skip("Skipping concurrent cache live tests by default. Uncomment this line to run live tests.")

	ctx := context.Background()

	t.Run("Concurrent requests for same token", func(t *testing.T) {
		// t.Skip("Uncomment to test concurrent same token requests")

		// Clear cache first
		ClearGlobalCache()
		SetGlobalCacheTTL(30 * time.Second)

		// Test concurrent requests for the same token (ETH)
		ethChains := []int{1, 42161, 8453} // Ethereum, Arbitrum, Base
		results := make(chan struct {
			chainID int
			price   float64
			err     error
		}, len(ethChains))

		// Start concurrent requests
		for _, chainID := range ethChains {
			go func(id int) {
				price, err := getTokenPriceUSD(ctx, id)
				results <- struct {
					chainID int
					price   float64
					err     error
				}{id, price, err}
			}(chainID)
		}

		// Collect results
		var prices []float64
		for i := 0; i < len(ethChains); i++ {
			result := <-results
			require.NoError(t, result.err, "Chain ID %d failed", result.chainID)
			require.Greater(t, result.price, 0.0, "Chain ID %d returned invalid price", result.chainID)
			prices = append(prices, result.price)
			t.Logf("Chain %d (ETH) price: $%.2f", result.chainID, result.price)
		}

		// All prices should be identical
		for i := 1; i < len(prices); i++ {
			assert.InDelta(t, prices[0], prices[i], 0.01, "ETH prices should be identical across chains")
		}

		// Verify cache stats
		count, _ := GetGlobalCacheStats()
		assert.Equal(t, 1, count, "Should only have 1 cached token despite multiple requests")
	})

	t.Run("Concurrent requests for different tokens", func(t *testing.T) {
		// t.Skip("Uncomment to test concurrent different token requests")

		// Clear cache first
		ClearGlobalCache()
		SetGlobalCacheTTL(30 * time.Second)

		// Test concurrent requests for different tokens
		chains := []int{1, 137, 56} // Ethereum, Polygon, BSC
		results := make(chan struct {
			chainID int
			price   float64
			err     error
		}, len(chains))

		// Start concurrent requests
		for _, chainID := range chains {
			go func(id int) {
				price, err := getTokenPriceUSD(ctx, id)
				results <- struct {
					chainID int
					price   float64
					err     error
				}{id, price, err}
			}(chainID)
		}

		// Collect results
		prices := make(map[int]float64)
		for i := 0; i < len(chains); i++ {
			result := <-results
			require.NoError(t, result.err, "Chain ID %d failed", result.chainID)
			require.Greater(t, result.price, 0.0, "Chain ID %d returned invalid price", result.chainID)
			prices[result.chainID] = result.price
			t.Logf("Chain %d price: $%.4f", result.chainID, result.price)
		}

		// Verify cache stats
		count, _ := GetGlobalCacheStats()
		assert.Equal(t, 3, count, "Should have 3 cached tokens (ethereum, matic-network, binancecoin)")
	})
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
