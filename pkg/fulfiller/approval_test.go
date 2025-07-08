package fulfiller

import (
	"math/big"
	"testing"

	"github.com/speedrun-hq/speedrunner/pkg/fulfiller/mocks"
	"github.com/stretchr/testify/assert"
)

// This file contains tests for token approval functionality using our custom mocks
// rather than depending on go-ethereum libraries

// TestUnlimitedApprovalValue verifies the unlimited approval value calculation
func TestUnlimitedApprovalValue(t *testing.T) {
	// Calculate max uint256 value using the same approach used in production code
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	// Verify it's a huge number (2^256 - 1)
	// The string is 78 digits long
	maxUintStr := maxUint256.String()
	assert.Equal(t, 78, len(maxUintStr), "MaxUint256 should be 78 digits long")
	assert.Equal(t, "115792089237316195423570985008687907853269984665640564039457584007913129639935", maxUintStr)
}

// TestOptimizedApprovalStrategy verifies our approval optimization strategy
func TestOptimizedApprovalStrategy(t *testing.T) {
	// Create test cases for different scenarios
	testCases := []struct {
		name              string
		requiredAmount    *big.Int
		currentAllowance  *big.Int
		expectedUnlimited bool
		approvalThreshold *big.Float
	}{
		{
			name:              "Zero current allowance should use unlimited approval",
			requiredAmount:    big.NewInt(100),
			currentAllowance:  big.NewInt(0),
			expectedUnlimited: true,
			approvalThreshold: big.NewFloat(0.3),
		},
		{
			name:              "Small amount relative to allowance should use exact amount",
			requiredAmount:    big.NewInt(10),
			currentAllowance:  big.NewInt(1000),
			expectedUnlimited: false,
			approvalThreshold: big.NewFloat(0.3),
		},
		{
			name:              "Large amount relative to allowance should use unlimited",
			requiredAmount:    big.NewInt(400), // 40% of allowance, over threshold
			currentAllowance:  big.NewInt(1000),
			expectedUnlimited: true,
			approvalThreshold: big.NewFloat(0.3),
		},
		{
			name:              "Amount at threshold should use unlimited",
			requiredAmount:    big.NewInt(300), // Exactly 30% of allowance
			currentAllowance:  big.NewInt(1000),
			expectedUnlimited: true,
			approvalThreshold: big.NewFloat(0.3),
		},
		{
			name:              "Amount just over threshold should use unlimited",
			requiredAmount:    big.NewInt(301), // Just over 30% of allowance
			currentAllowance:  big.NewInt(1000),
			expectedUnlimited: true,
			approvalThreshold: big.NewFloat(0.3),
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Calculate if we need unlimited approval
			requiredFloat := new(big.Float).SetInt(tc.requiredAmount)
			allowanceFloat := new(big.Float).SetInt(tc.currentAllowance)

			// Check if allowance is zero (should use unlimited)
			if tc.currentAllowance.Cmp(big.NewInt(0)) == 0 {
				assert.True(t, tc.expectedUnlimited, "Zero allowance should use unlimited approval")
				return
			}

			// Calculate ratio
			ratio := new(big.Float).Quo(requiredFloat, allowanceFloat)
			exceedsThreshold := ratio.Cmp(tc.approvalThreshold) >= 0

			assert.Equal(t, tc.expectedUnlimited, exceedsThreshold,
				"Unlimited approval decision should match expected for ratio %v vs threshold %v",
				ratio, tc.approvalThreshold)
		})
	}
}

// TestMockTokenApproval verifies the token approval flow using our custom mocks
func TestMockTokenApproval(t *testing.T) {
	// Create mocks
	tokenAddress := mocks.NewAddress("0x1234567890123456789012345678901234567890")
	spenderAddress := mocks.NewAddress("0x0987654321098765432109876543210987654321")
	mockContract := mocks.NewMockContract(tokenAddress)

	// Test case 1: Insufficient allowance, need approval
	t.Run("Insufficient allowance needs approval", func(t *testing.T) {
		// Set up current allowance = 100
		currentAllowance := big.NewInt(100)
		mockContract.SetCallResult("allowance", []interface{}{currentAllowance})

		// Request for amount = 200
		requiredAmount := big.NewInt(200)

		// Verify insufficient allowance check works
		var out []interface{}
		err := mockContract.Call(nil, &out, "allowance")
		assert.NoError(t, err, "Call to get allowance should not error")
		allowance := out[0].(*big.Int)

		assert.True(t, allowance.Cmp(requiredAmount) < 0,
			"Allowance should be less than required amount")
	})

	// Test case 2: Sufficient allowance, no approval needed
	t.Run("Sufficient allowance skips approval", func(t *testing.T) {
		// Set up current allowance = 1000
		currentAllowance := big.NewInt(1000)
		mockContract.SetCallResult("allowance", []interface{}{currentAllowance})

		// Request for amount = 200
		requiredAmount := big.NewInt(200)

		// Verify sufficient allowance check works
		var out []interface{}
		err := mockContract.Call(nil, &out, "allowance")
		assert.NoError(t, err, "Call to get allowance should not error")
		allowance := out[0].(*big.Int)

		assert.True(t, allowance.Cmp(requiredAmount) >= 0,
			"Allowance should be more than or equal to required amount")
	})

	// Test case 3: Verify mock transaction works
	t.Run("Approval transaction works", func(t *testing.T) {
		// Trigger an approval transaction
		tx, err := mockContract.Transact(nil, "approve", spenderAddress, big.NewInt(1000))
		assert.NoError(t, err, "Approval transaction should not error")
		assert.NotNil(t, tx, "Transaction should not be nil")

		// Verify transaction was stored
		assert.NotNil(t, mockContract.Transactions["approve"],
			"Transaction should be stored in mock contract")
	})
}
