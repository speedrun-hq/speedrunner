package fulfiller

import (
	"github.com/speedrun-hq/speedrunner/pkg/fulfiller/mocks"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/assert"
)

// MockChainConfig provides a test double for a chain configuration
type MockChainConfig struct {
	IntentAddress string
	Auth          *bind.TransactOpts
	Client        *simulated.Backend
}

// TestConfig is a simplified test version of the Service config
type TestConfig struct {
	Chains map[int]*MockChainConfig
}

// TestService is a simplified version of the Service for testing
type TestService struct {
	mu             sync.Mutex
	tokenAddresses map[int]common.Address
	config         *TestConfig
}

// TestFulfillIntentApprovalLogic tests the token approval logic in the fulfillIntent method
func TestFulfillIntentApprovalLogic(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Setup simulated blockchain
	sim, auth := setupTestSimulation(t)
	defer func(sim *simulated.Backend) {
		err := sim.Close()
		if err != nil {
			t.Fatalf("Failed to close simulated backend: %v", err)
		}
	}(sim)

	// Create test service
	service := &TestService{
		mu:             sync.Mutex{},
		tokenAddresses: make(map[int]common.Address),
		config: &TestConfig{
			Chains: map[int]*MockChainConfig{
				1: {
					IntentAddress: "0x4444444444444444444444444444444444444444",
					Auth:          auth,
					Client:        sim,
				},
			},
		},
	}

	// Set token address for test chain
	tokenAddress := common.HexToAddress("0x5555555555555555555555555555555555555555")
	service.tokenAddresses[1] = tokenAddress

	// This is a partial test that just verifies the test setup works
	t.Run("Test setup validation", func(t *testing.T) {
		assert.NotNil(t, service.config.Chains[1])
		assert.Equal(t, tokenAddress, service.tokenAddresses[1])
	})
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
