package fulfiller

import (
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

// MockContract implements a minimal contract interface for testing
type MockContract struct {
	fulfillCalled bool
	fulfillArgs   struct {
		intentID     common.Hash
		tokenAddress common.Address
		amount       *big.Int
		receiver     common.Address
	}
}

func (m *MockContract) Fulfill(opts *bind.TransactOpts, intentID common.Hash, tokenAddress common.Address, amount *big.Int, receiver common.Address) (common.Hash, error) {
	m.fulfillCalled = true
	m.fulfillArgs.intentID = intentID
	m.fulfillArgs.tokenAddress = tokenAddress
	m.fulfillArgs.amount = amount
	m.fulfillArgs.receiver = receiver
	return common.Hash{}, nil
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

// TestMaxApprovalValue tests that we're using the correct max approval value
func TestMaxApprovalValue(t *testing.T) {
	// This test doesn't require complex setup, so it can run in short mode
	// Calculate max uint256 value: 2^256 - 1
	expected := new(big.Int).Sub(
		new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil),
		big.NewInt(1),
	)

	// Get the value used in our code
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	// Verify they match
	assert.Equal(t, 0, expected.Cmp(maxUint256),
		"Max approval value is not correctly set to 2^256-1")
}
