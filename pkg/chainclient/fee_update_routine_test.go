package chainclient

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComputeWithdrawFee tests the ComputeWithdrawFee function with various inputs
func TestComputeWithdrawFee(t *testing.T) {
	tests := []struct {
		name           string
		gasPrice       *big.Int
		tokenPriceUSD  float64
		expectedFeeUSD float64
		description    string
	}{
		{
			name:           "Low gas price, low token price",
			gasPrice:       big.NewInt(20000000000), // 20 gwei
			tokenPriceUSD:  1000.0,                  // $1000 per token
			expectedFeeUSD: 2.0,                     // (20e9 * 100000) / 1e18 * 1000 = 2.0
			description:    "20 gwei gas price with $1000 token should result in $2.0 fee",
		},
		{
			name:           "High gas price, high token price",
			gasPrice:       big.NewInt(100000000000), // 100 gwei
			tokenPriceUSD:  5000.0,                   // $5000 per token
			expectedFeeUSD: 50.0,                     // (100e9 * 100000) / 1e18 * 5000 = 50.0
			description:    "100 gwei gas price with $5000 token should result in $50.0 fee",
		},
		{
			name:           "Very low gas price",
			gasPrice:       big.NewInt(1000000000), // 1 gwei
			tokenPriceUSD:  1.0,                    // $1 per token
			expectedFeeUSD: 0.0001,                 // (1e9 * 100000) / 1e18 * 1 = 0.0001
			description:    "1 gwei gas price with $1 token should result in $0.0001 fee",
		},
		{
			name:           "Zero gas price",
			gasPrice:       big.NewInt(0),
			tokenPriceUSD:  1000.0,
			expectedFeeUSD: 0.0,
			description:    "Zero gas price should result in zero fee",
		},
		{
			name:           "Zero token price",
			gasPrice:       big.NewInt(20000000000), // 20 gwei
			tokenPriceUSD:  0.0,
			expectedFeeUSD: 0.0,
			description:    "Zero token price should result in zero fee",
		},
		{
			name:           "Realistic Ethereum scenario",
			gasPrice:       big.NewInt(25000000000), // 25 gwei
			tokenPriceUSD:  3000.0,                  // $3000 per ETH
			expectedFeeUSD: 7.5,                     // (25e9 * 100000) / 1e18 * 3000 = 7.5
			description:    "Realistic Ethereum gas price and token price scenario",
		},
		{
			name:           "Very high gas price",
			gasPrice:       big.NewInt(1000000000000), // 1000 gwei
			tokenPriceUSD:  100.0,                     // $100 per token
			expectedFeeUSD: 10.0,                      // (1000e9 * 100000) / 1e18 * 100 = 10.0
			description:    "Very high gas price scenario",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeWithdrawFee(tt.gasPrice, tt.tokenPriceUSD)

			// Use approximate comparison for floating point values
			assert.InDelta(t, tt.expectedFeeUSD, result, 0.0001, tt.description)
		})
	}
}
