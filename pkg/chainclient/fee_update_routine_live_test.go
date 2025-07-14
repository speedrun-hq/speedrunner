package chainclient

import (
	"context"
	"testing"
	"time"

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

		price, err := GetTokenPriceUSD(ctx, 1)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 10000.0) // Reasonable upper bound for ETH price

		t.Logf("Ethereum price: $%.2f", price)
	})

	t.Run("Polygon", func(t *testing.T) {
		// t.Skip("Uncomment to test Polygon")

		price, err := GetTokenPriceUSD(ctx, 137)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 100.0) // Reasonable upper bound for MATIC price

		t.Logf("Polygon (MATIC) price: $%.4f", price)
	})

	t.Run("Arbitrum", func(t *testing.T) {
		// t.Skip("Uncomment to test Arbitrum")

		price, err := GetTokenPriceUSD(ctx, 42161)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 10000.0) // Reasonable upper bound for ETH price

		t.Logf("Arbitrum (ETH) price: $%.2f", price)
	})

	t.Run("BSC", func(t *testing.T) {
		// t.Skip("Uncomment to test BSC")

		price, err := GetTokenPriceUSD(ctx, 56)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 1000.0) // Reasonable upper bound for BNB price

		t.Logf("BSC (BNB) price: $%.2f", price)
	})

	t.Run("Avalanche", func(t *testing.T) {
		// t.Skip("Uncomment to test Avalanche")

		price, err := GetTokenPriceUSD(ctx, 43114)
		require.NoError(t, err)
		require.Greater(t, price, 0.0)
		require.Less(t, price, 1000.0) // Reasonable upper bound for AVAX price

		t.Logf("Avalanche (AVAX) price: $%.2f", price)
	})

	t.Run("Unsupported Chain ID", func(t *testing.T) {
		// t.Skip("Uncomment to test unsupported chain ID")

		_, err := GetTokenPriceUSD(ctx, 999999)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported chain ID")
	})

	t.Run("Context Timeout", func(t *testing.T) {
		// t.Skip("Uncomment to test context timeout")

		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()

		_, err := GetTokenPriceUSD(timeoutCtx, 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "context deadline exceeded")
	})
}

// TestGetTokenPriceUSD_Concurrent tests concurrent access to the API
func TestGetTokenPriceUSD_Concurrent(t *testing.T) {
	// Skip by default - uncomment to run live tests
	t.Skip("Skipping concurrent live tests by default. Uncomment this line to run live tests.")

	ctx := context.Background()
	chainIDs := []int{1, 137, 42161, 10, 56, 43114}

	// Test concurrent requests
	results := make(chan struct {
		chainID int
		price   float64
		err     error
	}, len(chainIDs))

	for _, chainID := range chainIDs {
		go func(id int) {
			price, err := GetTokenPriceUSD(ctx, id)
			results <- struct {
				chainID int
				price   float64
				err     error
			}{id, price, err}
		}(chainID)
	}

	// Collect results
	for i := 0; i < len(chainIDs); i++ {
		result := <-results
		require.NoError(t, result.err, "Chain ID %d failed", result.chainID)
		require.Greater(t, result.price, 0.0, "Chain ID %d returned invalid price", result.chainID)
		t.Logf("Chain %d price: $%.4f", result.chainID, result.price)
	}
}

// TestGetTokenPriceUSD_Performance tests the performance of the API calls
func TestGetTokenPriceUSD_Performance(t *testing.T) {
	// Skip by default - uncomment to run live tests
	t.Skip("Skipping performance live tests by default. Uncomment this line to run live tests.")

	ctx := context.Background()
	chainID := 1 // Ethereum

	// Measure response time
	start := time.Now()
	price, err := GetTokenPriceUSD(ctx, chainID)
	duration := time.Since(start)

	require.NoError(t, err)
	require.Greater(t, price, 0.0)
	require.Less(t, duration, 10*time.Second, "API call took too long: %v", duration)

	t.Logf("Ethereum price: $%.2f (fetched in %v)", price, duration)
}

// BenchmarkGetTokenPriceUSD benchmarks the GetTokenPriceUSD function
func BenchmarkGetTokenPriceUSD(b *testing.B) {
	// Skip by default - uncomment to run benchmarks
	b.Skip("Skipping benchmarks by default. Uncomment this line to run benchmarks.")

	ctx := context.Background()
	chainID := 1 // Ethereum

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetTokenPriceUSD(ctx, chainID)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}
