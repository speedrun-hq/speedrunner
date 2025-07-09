package fulfiller

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrunner/pkg/config"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// IntentFilter handles intent validation and filtering logic
type IntentFilter struct {
	config       *config.Config
	tokenManager *TokenManager
	logger       logger.Logger
}

// NewIntentFilter creates a new intent filter
func NewIntentFilter(config *config.Config, tokenManager *TokenManager, logger logger.Logger) *IntentFilter {
	return &IntentFilter{
		config:       config,
		tokenManager: tokenManager,
		logger:       logger,
	}
}

// FilterViableIntents filters intents that are viable for fulfillment
func (filter *IntentFilter) FilterViableIntents(intents []models.Intent, circuitBreakers map[int]*circuitbreaker.CircuitBreaker) []models.Intent {
	var viableIntents []models.Intent
	for _, intent := range intents {
		// Check circuit breaker status
		if breaker, exists := circuitBreakers[intent.DestinationChain]; exists {
			if breaker.IsOpen() {
				filter.logger.Info("Skipping intent %s: Circuit breaker is open for chain %d",
					intent.ID, intent.DestinationChain)
				continue
			}
		}

		// Check if source chain == destination chain
		if intent.SourceChain == intent.DestinationChain {
			filter.logger.Debug("Skipping intent %s: Source and destination chains are the same: %d",
				intent.ID, intent.SourceChain)
			continue
		}

		// Check if intent is more than 2 minutes old, only process recent intent
		// TODO: allow to configure this in config
		intentAge := time.Since(intent.CreatedAt)
		if intentAge > 2*time.Minute {
			filter.logger.Debug("Skipping intent %s: Intent is too old (age: %s)", intent.ID, intentAge.String())
			continue
		}

		// Check token balance
		if !filter.hasSufficientBalance(intent) {
			filter.logger.Debug("Skipping intent %s: Insufficient token balance for chain %d",
				intent.ID, intent.DestinationChain)
			continue
		}

		fee, success := new(big.Int).SetString(intent.IntentFee, 10)
		if !success {
			filter.logger.Debug("Skipping intent %s: Error parsing intent fee: invalid format", intent.ID)
			continue
		}
		if fee.Cmp(big.NewInt(0)) <= 0 {
			filter.logger.Debug("Skipping intent %s: Fee is zero or negative", intent.ID)
			continue
		}

		// Check if fee meets minimum requirement for the chain
		destinationChainConfig, destinationExists := filter.config.Chains[intent.DestinationChain]

		if !destinationExists {
			filter.logger.Debug("Skipping intent %s: Chain configuration not found for %d",
				intent.ID, intent.DestinationChain)
			continue
		}

		// convert fee for BSC unit difference
		if intent.SourceChain == 56 {
			fee = new(big.Int).Div(fee, big.NewInt(1000000000000))
		} else if intent.DestinationChain == 56 {
			fee = new(big.Int).Mul(fee, big.NewInt(1000000000000))
		}

		// Check if fee meets minimum requirement for the chain
		if destinationChainConfig.MinFee != nil && fee.Cmp(destinationChainConfig.MinFee) < 0 {
			filter.logger.Debug("Skipping intent %s: Fee %s below minimum %s for chain %d",
				intent.ID, fee.String(), destinationChainConfig.MinFee.String(), intent.DestinationChain)
			continue
		}

		viableIntents = append(viableIntents, intent)
	}
	return viableIntents
}

// HasSufficientBalance checks if we have sufficient token balance for the intent
func (filter *IntentFilter) hasSufficientBalance(intent models.Intent) bool {
	// Get token address from the intent
	tokenAddress := common.HexToAddress(intent.Token)

	// Get token type from address
	tokenType := filter.getTokenTypeFromAddress(tokenAddress)
	if tokenType == "" {
		filter.logger.DebugWithChain(intent.DestinationChain, "Unknown token type for address %s", tokenAddress.Hex())
		return false
	}

	// Get token for the destination chain
	token, exists := filter.tokenManager.GetToken(intent.DestinationChain, tokenType)
	if !exists {
		filter.logger.DebugWithChain(intent.DestinationChain, "Token %s not configured", tokenType)
		return false
	}

	// Get token balance
	balance, err := filter.getTokenBalance(intent.DestinationChain, token.Address)
	if err != nil {
		filter.logger.DebugWithChain(intent.DestinationChain, "Error getting token balance: %v", err)
		return false
	}

	// Convert intent amount to big.Int
	amount, success := new(big.Int).SetString(intent.Amount, 10)
	if !success {
		filter.logger.DebugWithChain(intent.DestinationChain, "Error parsing intent amount: %s", intent.Amount)
		return false
	}

	// convert amount for BSC unit difference
	if intent.SourceChain == 56 {
		amount = new(big.Int).Div(amount, big.NewInt(1000000000000))
	} else if intent.DestinationChain == 56 {
		amount = new(big.Int).Mul(amount, big.NewInt(1000000000000))
	}

	// Check if we have sufficient balance
	amountFloat := new(big.Float).SetInt(amount)
	return balance.Cmp(amountFloat) >= 0
}

// GetTokenBalance gets the token balance for a given chain and token address
func (filter *IntentFilter) getTokenBalance(chainID int, tokenAddress common.Address) (*big.Float, error) {
	chainConfig, exists := filter.config.Chains[chainID]
	if !exists {
		return nil, fmt.Errorf("chain configuration not found for chain %d", chainID)
	}

	// Create ERC20 contract instance
	token, err := contracts.NewERC20(tokenAddress, chainConfig.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create ERC20 contract: %v", err)
	}

	// Get raw balance
	rawBalance, err := token.BalanceOf(nil, common.HexToAddress(filter.config.FulfillerAddress))
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance: %v", err)
	}

	// Normalize balance by dividing by 10^decimals
	decimals, err := token.Decimals(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get token decimals: %v", err)
	}

	// Convert to big.Float for precision
	balanceFloat := new(big.Float).SetInt(rawBalance)

	// Calculate divisor based on decimals
	divisor := new(big.Float).SetFloat64(1.0)
	for i := uint8(0); i < decimals; i++ {
		divisor = new(big.Float).Mul(divisor, big.NewFloat(10.0))
	}

	normalizedBalance := new(big.Float).Quo(balanceFloat, divisor)
	return normalizedBalance, nil
}

// GetTokenTypeFromAddress gets the token type from an address
func (filter *IntentFilter) getTokenTypeFromAddress(address common.Address) TokenType {
	// Use the token manager's method to get token type from address
	return filter.tokenManager.GetTokenTypeFromAddress(address)
}
