package blockchain

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/contracts"
)

// ChainConfig holds the configuration for a specific chain
type ChainConfig struct {
	ChainID       int
	RPCURL        string
	IntentAddress string
	Client        *ethclient.Client
	Contract      *contracts.Intent
	Auth          *bind.TransactOpts
	MinFee        *big.Int
	GasMultiplier float64
}

// NewChainConfig creates a chain configuration from environment variables
func NewChainConfig(chainID int, rpcURL string, intentAddress string, minFee string) *ChainConfig {
	minFeeBig := big.NewInt(0)
	if minFee != "" {
		var success bool
		minFeeBig, success = new(big.Int).SetString(minFee, 10)
		if !success {
			log.Printf("Warning: Invalid min fee format for chain %d: %s", chainID, minFee)
			minFeeBig = big.NewInt(0)
		}
	}

	// Get gas multiplier from environment, default to 1.1
	gasMultiplierStr := os.Getenv(fmt.Sprintf("CHAIN_%d_GAS_MULTIPLIER", chainID))
	gasMultiplier := 1.1 // default gas multiplier (10% buffer)
	if gasMultiplierStr != "" {
		parsedMultiplier, err := strconv.ParseFloat(gasMultiplierStr, 64)
		if err == nil && parsedMultiplier > 0 {
			gasMultiplier = parsedMultiplier
		}
	}

	return &ChainConfig{
		ChainID:       chainID,
		RPCURL:        rpcURL,
		IntentAddress: intentAddress,
		MinFee:        minFeeBig,
		GasMultiplier: gasMultiplier,
	}
}

// Connect establishes connections to blockchain RPC and initializes contract instances
func (c *ChainConfig) Connect(privateKey string) error {
	// Connect to Ethereum client
	client, err := ethclient.Dial(c.RPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to client: %v", err)
	}
	c.Client = client

	// Set up authenticator and contract binding
	if privateKey != "" {
		auth, err := createAuthenticator(client, privateKey)
		if err != nil {
			return fmt.Errorf("failed to create authenticator: %v", err)
		}
		c.Auth = auth
	}

	// Initialize contract binding
	contract, err := contracts.NewIntent(common.HexToAddress(c.IntentAddress), client)
	if err != nil {
		return fmt.Errorf("failed to initialize contract: %v", err)
	}
	c.Contract = contract

	return nil
}

// UpdateGasPrice updates the gas price based on current network conditions
func (c *ChainConfig) UpdateGasPrice(ctx context.Context) (*big.Int, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("client not connected")
	}

	// Get current gas price from the network
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	gasPrice, err := c.Client.SuggestGasPrice(timeoutCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %v", err)
	}

	// Apply gas multiplier (e.g. 1.1 = 10% buffer)
	multipliedGasPrice := new(big.Float).Mul(
		new(big.Float).SetInt(gasPrice),
		big.NewFloat(c.GasMultiplier),
	)

	// Convert back to big.Int
	finalGasPrice := new(big.Int)
	multipliedGasPrice.Int(finalGasPrice)

	// Update the auth with the new gas price
	if c.Auth != nil {
		c.Auth.GasPrice = finalGasPrice
	}

	log.Printf("Updated gas price for chain %d: %s wei (multiplier: %.2f)",
		c.ChainID, finalGasPrice.String(), c.GasMultiplier)

	return finalGasPrice, nil
}

// GetLatestBlockNumber gets the latest block number from the chain
func (c *ChainConfig) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	if c.Client == nil {
		return 0, fmt.Errorf("client not connected")
	}

	return c.Client.BlockNumber(ctx)
}

// Helper function to create authenticator
func createAuthenticator(client *ethclient.Client, privateKeyHex string) (*bind.TransactOpts, error) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Get chain ID
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %v", err)
	}

	// Create transaction signer
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}

	return auth, nil
}
