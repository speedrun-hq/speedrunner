package fulfiller

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// NonceManagedFulfillment contains methods to execute transactions with nonce management
// This is used by the worker to process intents with proper nonce handling

// GetNonceAndTrackTx gets a nonce for the given chain and tracks the transaction
func (s *Service) GetNonceAndTrackTx(ctx context.Context, chainID int, txHash common.Hash) (uint64, error) {
	// Get chain config
	s.mu.Lock()
	chainConfig, exists := s.config.Chains[chainID]
	s.mu.Unlock()

	if !exists {
		return 0, fmt.Errorf("chain configuration not found for: %d", chainID)
	}

	// Get nonce for the transaction
	nonce, err := s.nonceManager.GetNonce(ctx, chainID, chainConfig.Client, chainConfig.Auth.From)
	if err != nil {
		return 0, fmt.Errorf("failed to get nonce: %v", err)
	}

	// Track the transaction
	s.nonceManager.TrackTransaction(chainID, txHash, chainConfig.Auth.From, nonce)

	return nonce, nil
}

// SetNonceInTxOpts sets the nonce in the transaction options
func (s *Service) SetNonceInTxOpts(chainID int, nonce uint64, txOpts *bind.TransactOpts) {
	// Set nonce for the transaction
	txOpts.Nonce = big.NewInt(int64(nonce))
}

// MarkTransactionFinished handles transaction completion in the nonce manager
func (s *Service) MarkTransactionFinished(chainID int, txHash common.Hash, success bool) {
	if success {
		if err := s.nonceManager.MarkTransactionConfirmed(chainID, txHash); err != nil {
			s.logger.InfoWithChain(chainID, "Warning: Failed to mark transaction %s as confirmed: %v", txHash.Hex(), err)
		}
	} else {
		if err := s.nonceManager.MarkTransactionFailed(chainID, txHash); err != nil {
			s.logger.InfoWithChain(chainID, "Warning: Failed to mark transaction %s as failed: %v", txHash.Hex(), err)
		}
	}
}
