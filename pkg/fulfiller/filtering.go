package fulfiller

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/chains"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// filterViableIntents filters intents that are viable for fulfillment
func (s *Service) filterViableIntents(intents []models.Intent) []models.Intent {
	var viableIntents []models.Intent
	for _, intent := range intents {
		// Check circuit breaker status
		if breaker, exists := s.circuitBreakers[intent.DestinationChain]; exists {
			if breaker.IsOpen() {
				s.logger.Info("Skipping intent %s: Circuit breaker is open for chain %d",
					intent.ID, intent.DestinationChain)
				continue
			}
		}

		// Check if source chain == destination chain
		if intent.SourceChain == intent.DestinationChain {
			s.logger.Debug("Skipping intent %s: Source and destination chains are the same: %d",
				intent.ID, intent.SourceChain)
			continue
		}

		// Check if intent is more than 2 minutes old, only process recent intent
		// TODO: allow to configure this in config
		intentAge := time.Since(intent.CreatedAt)
		if intentAge > 2*time.Minute {
			s.logger.Debug("Skipping intent %s: Intent is too old (age: %s)", intent.ID, intentAge.String())
			continue
		}

		// Check token balance
		if !s.hasSufficientBalance(intent) {
			s.logger.Debug("Skipping intent %s: Insufficient token balance for chain %d",
				intent.ID, intent.DestinationChain)
			continue
		}

		fee, success := new(big.Int).SetString(intent.IntentFee, 10)
		if !success {
			s.logger.Debug("Skipping intent %s: Error parsing intent fee: invalid format", intent.ID)
			continue
		}
		if fee.Cmp(big.NewInt(0)) <= 0 {
			s.logger.Debug("Skipping intent %s: Fee is zero or negative", intent.ID)
			continue
		}

		// Check if fee meets minimum requirement for the chain
		s.mu.Lock()
		destinationChainClient, destinationExists := s.chainClients[intent.DestinationChain]
		s.mu.Unlock()

		if !destinationExists {
			s.logger.Debug("Skipping intent %s: Chain configuration not found for %d",
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
		if destinationChainClient.MinFee != nil && fee.Cmp(destinationChainClient.MinFee) < 0 {
			s.logger.Debug("Skipping intent %s: Fee %s below minimum %s for chain %d",
				intent.ID, fee.String(), destinationChainClient.MinFee.String(), intent.DestinationChain)
			continue
		}

		viableIntents = append(viableIntents, intent)
	}
	return viableIntents
}

// hasSufficientBalance checks if we have sufficient token balance for the intent
func (s *Service) hasSufficientBalance(intent models.Intent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get token type from address
	tokenType := chains.GetTokenType(intent.Token)
	if tokenType == "" {
		s.logger.DebugWithChain(intent.DestinationChain, "Unknown token type for address %s", intent.Token)
		return false
	}

	// Get token for the destination chain
	token := chains.GetTokenEthAddress(intent.DestinationChain, tokenType)
	if token == (common.Address{}) {
		s.logger.DebugWithChain(intent.DestinationChain, "Invalid token address for %s", tokenType)
		return false
	}

	// Get token balance
	balance, err := s.getTokenBalance(intent.DestinationChain, token)
	if err != nil {
		s.logger.DebugWithChain(intent.DestinationChain, "Error getting token balance: %v", err)
		return false
	}

	// Convert intent amount to big.Int
	amount, success := new(big.Int).SetString(intent.Amount, 10)
	if !success {
		s.logger.DebugWithChain(intent.DestinationChain, "Error parsing intent amount: %s", intent.Amount)
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
