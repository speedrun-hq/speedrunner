package testutil

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Constants for testing
const (
	DefaultTestTimeout = 5 * time.Second
)

// SetupSimulation creates a simulated blockchain environment for testing
func SetupSimulation(t *testing.T) (*simulated.Backend, *bind.TransactOpts, common.Address) {
	// Generate a new random private key
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err, "Failed to generate private key")

	// Create auth
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(1))
	require.NoError(t, err, "Failed to create transactor")

	// Fund the account with some initial balance
	balance := new(big.Int)
	balance.SetString("10000000000000000000", 10) // 10 ETH
	address := auth.From
	//nolint:SA1019 // Using deprecated GenesisAccount for compatibility
	genesisAlloc := map[common.Address]core.GenesisAccount{
		address: {
			Balance: balance,
		},
	}

	// Create simulated blockchain
	sim := simulated.NewBackend(genesisAlloc)

	return sim, auth, address
}

// GenerateAddress creates a random address for testing
func GenerateAddress() common.Address {
	privateKey, _ := crypto.GenerateKey()
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}

// CreateBigInt parses a string into a big.Int
func CreateBigInt(value string) *big.Int {
	result := new(big.Int)
	result.SetString(value, 10)
	return result
}

// AssertBigIntEqual compares two big.Int values for equality in tests
func AssertBigIntEqual(t *testing.T, expected, actual *big.Int, msgAndArgs ...interface{}) {
	if expected == nil && actual == nil {
		return
	}

	if (expected == nil && actual != nil) || (expected != nil && actual == nil) {
		assert.Fail(t, "Values not equal", msgAndArgs...)
		return
	}

	assert.Equal(t, 0, expected.Cmp(actual), msgAndArgs...)
}

// SetupTestWithTimeout creates a test with a timeout
func SetupTestWithTimeout(t *testing.T) (func(), context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTestTimeout)

	cleanup := func() {
		cancel()
	}

	return cleanup, ctx, cancel
}
