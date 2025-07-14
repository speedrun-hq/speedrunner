package chainclient

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
)

// Client contains client and config information for a specific blockchain
type Client struct {
	Ctx            context.Context
	ChainID        int
	RPCURL         string
	IntentAddress  string
	MinFee         *big.Int
	MaxGasPrice    *big.Int
	Client         *ethclient.Client
	IntentContract *contracts.Intent
	Auth           *bind.TransactOpts
	GasMultiplier  float64
}

// New creates a new client
// TODO: should return error for invalid values to avoid unexpected behavior
func New(ctx context.Context, chainID int, rpcURL string, intentAddress string, minFee string, privateKey string) (*Client, error) {
	minFeeBig := big.NewInt(0)
	if minFee != "" {
		var success bool
		minFeeBig, success = new(big.Int).SetString(minFee, 10)
		if !success {
			return nil, fmt.Errorf("invalid minFee value: %s", minFee)
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

	// Connect to the chain using the provided RPC URL
	client := &Client{
		Ctx:           ctx,
		ChainID:       chainID,
		RPCURL:        rpcURL,
		IntentAddress: intentAddress,
		MinFee:        minFeeBig,
		GasMultiplier: gasMultiplier,
	}
	if err := client.connect(ctx, privateKey); err != nil {
		return nil, fmt.Errorf("failed to connect to chain %d: %v", chainID, err)
	}

	return client, nil
}

// UpdateGasPrice updates the gas price based on current network conditions
func (c *Client) UpdateGasPrice(ctx context.Context) (*big.Int, error) {
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

	return finalGasPrice, nil
}

// GetLatestBlockNumber gets the latest block number from the chain
func (c *Client) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	if c.Client == nil {
		return 0, fmt.Errorf("client not connected")
	}

	return c.Client.BlockNumber(ctx)
}

// connect establishes connections to blockchain RPC and initializes contract instances
func (c *Client) connect(ctx context.Context, privateKey string) error {
	// Connect to Ethereum client
	client, err := ethclient.Dial(c.RPCURL)
	if err != nil {
		return fmt.Errorf("failed to connect to client: %v", err)
	}
	c.Client = client

	// Set up authenticator and contract binding
	if privateKey != "" {
		auth, err := createAuthenticator(ctx, client, privateKey)
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
	c.IntentContract = contract

	return nil
}

// Helper function to create authenticator
func createAuthenticator(ctx context.Context, client *ethclient.Client, privateKeyHex string) (*bind.TransactOpts, error) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Get chain ID
	chainID, err := client.ChainID(ctx)
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
