package fulfiller

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/metrics"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/models"
)

// fulfillIntent attempts to fulfill a single intent
func (s *Service) fulfillIntent(intent models.Intent) error {
	s.mu.Lock()
	chainConfig, exists := s.config.Chains[intent.DestinationChain]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("destination chain configuration not found for: %d", intent.DestinationChain)
	}

	// Update gas price before transaction
	finalGasPrice, err := chainConfig.UpdateGasPrice(context.Background())
	if err != nil {
		log.Printf("Warning: Failed to update gas price for chain %d: %v", intent.DestinationChain, err)
		// Continue with default/previous gas price
	} else {
		// Update metric (convert to gwei for readability)
		gasPriceGwei := new(big.Float).Quo(
			new(big.Float).SetInt(finalGasPrice),
			big.NewFloat(1e9), // 1 gwei = 10^9 wei
		)
		gweiFlt, _ := gasPriceGwei.Float64()
		metrics.GasPrice.WithLabelValues(fmt.Sprintf("%d", intent.DestinationChain)).Set(gweiFlt)
		log.Printf("Updated gas price for chain %d: %.2f gwei", intent.DestinationChain, gweiFlt)
	}

	// Convert intent ID to bytes32
	intentID := common.HexToHash(intent.ID)

	// Convert amount to big.Int
	amount, ok := new(big.Int).SetString(intent.Amount, 10)
	if !ok {
		return fmt.Errorf("invalid amount: %s", intent.Amount)
	}

	// convert for BSC unit difference
	if intent.SourceChain == 56 {
		amount = new(big.Int).Div(amount, big.NewInt(1000000000000))
	} else if intent.DestinationChain == 56 {
		amount = new(big.Int).Mul(amount, big.NewInt(1000000000000))
	}

	log.Printf("Fulfilling intent %s on chain %d with amount %s", intent.ID, intent.DestinationChain, amount.String())

	// Convert addresses
	receiver := common.HexToAddress(intent.Recipient)

	// Get the Intent contract address
	intentAddress := common.HexToAddress(chainConfig.IntentAddress)

	// Get the token type from intent, default to USDC if not specified
	tokenType := TokenTypeUSDC
	if intent.TokenType != "" {
		tokenType = TokenType(strings.ToUpper(intent.TokenType))
	}

	// Get token address from the map
	s.mu.Lock()
	chainTokens, exists := s.tokens[intent.DestinationChain]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("token mapping not configured for chain: %d", intent.DestinationChain)
	}

	token, exists := chainTokens[tokenType]
	if !exists {
		return fmt.Errorf("token type %s not configured for chain: %d", tokenType, intent.DestinationChain)
	}

	tokenAddress := token.Address
	log.Printf("Using token %s (%s) address %s for chain %d", token.Symbol, tokenType, tokenAddress.Hex(), intent.DestinationChain)

	// Apply current gas price to transactor
	s.mu.Lock()
	txOpts := *chainConfig.Auth
	s.mu.Unlock()

	// Check token allowance from cache or blockchain
	ctx := context.Background()

	// Use our helper function to check and cache allowance
	hasAllowance, err := s.checkAndCacheAllowance(
		ctx,
		chainConfig,
		tokenAddress,
		txOpts.From,
		intentAddress,
		amount,
	)
	if err != nil {
		log.Printf("Failed to check allowance for intent %s: %v", intent.ID, err)
		// Continue with default behavior (try approval)
		hasAllowance = false
	}

	// Proceed with approval if needed
	if !hasAllowance {
		log.Printf("Initiating token approval for intent %s on chain %d (token: %s, spender: %s)",
			intent.ID, intent.DestinationChain, tokenAddress.Hex(), intentAddress.Hex())

		// Get ERC20 ABI
		erc20ABI, err := getERC20ABI()
		if err != nil {
			return fmt.Errorf("failed to parse ERC20 ABI: %v", err)
		}

		// Create ERC20 contract binding
		erc20Contract := bind.NewBoundContract(
			tokenAddress,
			erc20ABI,
			chainConfig.Client,
			chainConfig.Client,
			chainConfig.Client,
		)

		// Get nonce for approval transaction
		approvalNonce, err := s.nonceManager.GetNonce(ctx, intent.DestinationChain, chainConfig.Client, txOpts.From)
		if err != nil {
			return fmt.Errorf("failed to get nonce for approval: %v", err)
		}

		// Set nonce for approval transaction
		txOpts.Nonce = big.NewInt(int64(approvalNonce))

		// Use max uint256 value for unlimited approval to avoid future approval transactions
		maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

		// Send the approve transaction with unlimited amount
		approveTx, err := erc20Contract.Transact(&txOpts, "approve", intentAddress, maxUint256)
		if err != nil {
			log.Printf("Failed to create approval transaction for intent %s: %v", intent.ID, err)
			return fmt.Errorf("failed to approve token transfer: %v", err)
		}

		// Track the approval transaction
		s.nonceManager.TrackTransaction(intent.DestinationChain, approveTx.Hash(), approvalNonce)

		log.Printf("Approval transaction sent for intent %s: %s (nonce: %d)",
			intent.ID, approveTx.Hash().Hex(), approvalNonce)

		// Wait for the approve transaction to be mined
		approveReceipt, err := bind.WaitMined(ctx, chainConfig.Client, approveTx)
		if err != nil {
			log.Printf("Failed to mine approval transaction for intent %s: %v", intent.ID, err)
			// Don't return error yet, mark as failed in nonce manager
			s.nonceManager.MarkTransactionFailed(intent.DestinationChain, approvalNonce)
			return fmt.Errorf("failed to wait for approve transaction: %v", err)
		}

		if approveReceipt.Status == 0 {
			log.Printf("Approval transaction failed for intent %s: %s", intent.ID, approveTx.Hash().Hex())
			s.nonceManager.MarkTransactionFailed(intent.DestinationChain, approvalNonce)
			return fmt.Errorf("approve transaction failed")
		}

		// Mark transaction as confirmed
		s.nonceManager.MarkTransactionConfirmed(intent.DestinationChain, approvalNonce)

		log.Printf("Approval successful for intent %s: %s (gas used: %d)",
			intent.ID, approveTx.Hash().Hex(), approveReceipt.GasUsed)

		// Update our allowance cache with the new approval
		s.updateAllowanceCache(
			intent.DestinationChain,
			tokenAddress,
			txOpts.From,
			intentAddress,
			maxUint256,
		)
	}

	// Get nonce for fulfillment transaction
	fulfillNonce, err := s.nonceManager.GetNonce(ctx, intent.DestinationChain, chainConfig.Client, txOpts.From)
	if err != nil {
		return fmt.Errorf("failed to get nonce for fulfillment: %v", err)
	}

	// Set nonce for fulfillment transaction
	txOpts.Nonce = big.NewInt(int64(fulfillNonce))

	// Now call the contract's fulfill function with current gas price
	log.Printf("Initiating fulfillment for intent %s on chain %d (token: %s, amount: %s, receiver: %s, nonce: %d)",
		intent.ID, intent.DestinationChain, tokenAddress.Hex(), amount.String(), receiver.Hex(), fulfillNonce)

	tx, err := chainConfig.Contract.Fulfill(&txOpts, intentID, tokenAddress, amount, receiver)
	if err != nil {
		log.Printf("Failed to create fulfillment transaction for intent %s: %v", intent.ID, err)
		return fmt.Errorf("failed to fulfill intent on %d: %v", intent.DestinationChain, err)
	}

	// Track the fulfillment transaction
	s.nonceManager.TrackTransaction(intent.DestinationChain, tx.Hash(), fulfillNonce)

	log.Printf("Fulfillment transaction sent for intent %s: %s (nonce: %d)",
		intent.ID, tx.Hash().Hex(), fulfillNonce)

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, chainConfig.Client, tx)
	if err != nil {
		log.Printf("Failed to mine fulfillment transaction for intent %s: %v", intent.ID, err)
		s.nonceManager.MarkTransactionFailed(intent.DestinationChain, fulfillNonce)
		return fmt.Errorf("failed to wait for transaction on %d: %v", intent.DestinationChain, err)
	}

	if receipt.Status == 0 {
		log.Printf("Fulfillment transaction failed for intent %s: %s", intent.ID, tx.Hash().Hex())
		s.nonceManager.MarkTransactionFailed(intent.DestinationChain, fulfillNonce)
		return fmt.Errorf("transaction failed on %d", intent.DestinationChain)
	}

	// Mark transaction as confirmed
	s.nonceManager.MarkTransactionConfirmed(intent.DestinationChain, fulfillNonce)

	log.Printf("Fulfillment successful for intent %s: %s (gas used: %d)",
		intent.ID, tx.Hash().Hex(), receipt.GasUsed)
	return nil
}
