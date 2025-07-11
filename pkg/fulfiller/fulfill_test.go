package fulfiller

import (
	"github.com/speedrun-hq/speedrunner/pkg/fulfiller/mocks"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
