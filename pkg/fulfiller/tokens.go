package fulfiller

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
)

// getTokenBalance gets the token balance for a given chain and token address
func (s *Service) getTokenBalance(chainID int, tokenAddress common.Address) (*big.Float, error) {
	chainClient, exists := s.chainClients[chainID]
	if !exists {
		return nil, fmt.Errorf("chain client not found for chain %d", chainID)
	}

	// Create ERC20 contract instance
	token, err := contracts.NewERC20(tokenAddress, chainClient.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create ERC20 contract: %v", err)
	}

	// Get raw balance
	rawBalance, err := token.BalanceOf(nil, common.HexToAddress(s.config.FulfillerAddress))
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance: %v", err)
	}

	// Normalize balance by dividing by 10^decimals
	balanceFloat := new(big.Float).SetInt(rawBalance)

	return balanceFloat, nil
}
