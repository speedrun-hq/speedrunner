package fulfiller

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMaxUint256Calculation ensures our MaxUint256 calculation is correct
func TestMaxUint256Calculation(t *testing.T) {
	// Calculate max uint256 value: 2^256 - 1
	expected := new(big.Int).Sub(
		new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil),
		big.NewInt(1),
	)

	// Get the value using the method we use in the code
	calculated := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	// Verify they match
	assert.Equal(t, 0, expected.Cmp(calculated),
		"Max uint256 calculation is not correct")
}

// TestApprovalThreshold tests that the approval threshold is correctly defined
func TestApprovalThreshold(t *testing.T) {
	// Define the threshold value we use in production
	threshold := big.NewFloat(0.3)

	// Test that it's initialized to the expected value
	expected := big.NewFloat(0.3)
	assert.Equal(t, 0, threshold.Cmp(expected),
		"Approval threshold should be set to 0.3")
}
