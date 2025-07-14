package chains

import (
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestGetStandardizedAmount(t *testing.T) {
	// Helper function for creating big.Int from string
	setString := func(s string) *big.Int {
		bigInt, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("Failed to set string %s to big.Int", s)
		}
		return bigInt
	}

	tests := []struct {
		name        string
		baseAmount  *big.Int
		chainID     int
		tokenType   TokenType
		expected    float64
		description string
		isErr       bool
	}{
		// USDC tests - 6 decimals (non-BSC chains)
		{
			name:        "USDC_Ethereum_1_token",
			baseAmount:  big.NewInt(1000000), // 1 USDC with 6 decimals
			chainID:     1,                   // Ethereum
			tokenType:   TokenTypeUSDC,
			expected:    1.0,
			description: "1 USDC on Ethereum should return 1.0",
		},
		{
			name:        "USDC_Ethereum_half_token",
			baseAmount:  big.NewInt(500000), // 0.5 USDC with 6 decimals
			chainID:     1,
			tokenType:   TokenTypeUSDC,
			expected:    0.5,
			description: "0.5 USDC on Ethereum should return 0.5",
		},
		{
			name:        "USDC_Polygon_100_tokens",
			baseAmount:  big.NewInt(100000000), // 100 USDC with 6 decimals
			chainID:     137,                   // Polygon
			tokenType:   TokenTypeUSDC,
			expected:    100.0,
			description: "100 USDC on Polygon should return 100.0",
		},

		// USDC tests - 18 decimals (BSC chain)
		{
			name:        "USDC_BSC_1_token",
			baseAmount:  setString("1000000000000000000"), // 1 USDC with 18 decimals
			chainID:     56,                               // BSC
			tokenType:   TokenTypeUSDC,
			expected:    1.0,
			description: "1 USDC on BSC should return 1.0",
		},
		{
			name:        "USDC_BSC_1000_tokens",
			baseAmount:  setString("1000000000000000000000"), // 1000 USDC with 18 decimals
			chainID:     56,
			tokenType:   TokenTypeUSDC,
			expected:    1000.0,
			description: "1000 USDC on BSC should return 1000.0",
		},

		// USDT tests - 6 decimals (non-BSC chains)
		{
			name:        "USDT_Ethereum_1_token",
			baseAmount:  big.NewInt(1000000), // 1 USDT with 6 decimals
			chainID:     1,                   // Ethereum
			tokenType:   TokenTypeUSDT,
			expected:    1.0,
			description: "1 USDT on Ethereum should return 1.0",
		},
		{
			name:        "USDT_Arbitrum_250_tokens",
			baseAmount:  big.NewInt(250000000), // 250 USDT with 6 decimals
			chainID:     42161,                 // Arbitrum
			tokenType:   TokenTypeUSDT,
			expected:    250.0,
			description: "250 USDT on Arbitrum should return 250.0",
		},

		// USDT tests - 18 decimals (BSC chain)
		{
			name:        "USDT_BSC_1_token",
			baseAmount:  setString("1000000000000000000"), // 1 USDT with 18 decimals
			chainID:     56,                               // BSC
			tokenType:   TokenTypeUSDT,
			expected:    1.0,
			description: "1 USDT on BSC should return 1.0",
		},
		{
			name:        "USDT_BSC_small_amount",
			baseAmount:  setString("100000000000000000"), // 0.1 USDT with 18 decimals
			chainID:     56,
			tokenType:   TokenTypeUSDT,
			expected:    0.1,
			description: "0.1 USDT on BSC should return 0.1",
		},

		// Edge cases - nil and zero amounts
		{
			name:        "nil_amount",
			baseAmount:  nil,
			chainID:     1,
			tokenType:   TokenTypeUSDC,
			expected:    0.0,
			description: "nil amount should return 0.0",
			isErr:       true, // Expect an error for nil amount
		},
		{
			name:        "zero_amount",
			baseAmount:  big.NewInt(0),
			chainID:     1,
			tokenType:   TokenTypeUSDC,
			expected:    0.0,
			description: "zero amount should return 0.0",
			isErr:       true, // Expect an error for zero amount
		},
		{
			name:        "negative_amount",
			baseAmount:  big.NewInt(-1000000),
			chainID:     1,
			tokenType:   TokenTypeUSDC,
			expected:    0.0,
			description: "negative amount should return 0.0",
			isErr:       true, // Expect an error for negative amount
		},

		// Very small amounts (testing precision)
		{
			name:        "USDC_very_small_amount",
			baseAmount:  big.NewInt(1), // 0.000001 USDC with 6 decimals
			chainID:     1,
			tokenType:   TokenTypeUSDC,
			expected:    0.000001,
			description: "very small USDC amount should be handled correctly",
		},
		{
			name:        "USDT_BSC_very_small_amount",
			baseAmount:  setString("1000000000000"), // 0.000001 USDT with 18 decimals
			chainID:     56,
			tokenType:   TokenTypeUSDT,
			expected:    0.000001,
			description: "very small USDT amount on BSC should be handled correctly",
		},

		// Large amounts
		{
			name:        "USDC_large_amount",
			baseAmount:  setString("1000000000000"), // 1,000,000 USDC with 6 decimals
			chainID:     1,
			tokenType:   TokenTypeUSDC,
			expected:    1000000.0,
			description: "large USDC amount should be handled correctly",
		},
		{
			name:        "USDT_BSC_large_amount",
			baseAmount:  setString("1000000000000000000000000"), // 1,000,000 USDT with 18 decimals
			chainID:     56,
			tokenType:   TokenTypeUSDT,
			expected:    1000000.0,
			description: "large USDT amount on BSC should be handled correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetStandardizedAmount(tt.baseAmount, tt.chainID, tt.tokenType)
			if tt.isErr {
				require.Error(t, err, "Expected an error for test: %s", tt.description)
				return
			} else {
				require.NoError(t, err, "Unexpected error for test: %s", tt.description)
			}

			// Use a small epsilon for floating point comparison
			const epsilon = 1e-9
			if abs(result-tt.expected) > epsilon {
				t.Errorf("GetStandardizedAmount() = %v, expected %v (difference: %v)\nTest: %s",
					result, tt.expected, abs(result-tt.expected), tt.description)
			}
		})
	}
}

// Helper function for absolute value of float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
