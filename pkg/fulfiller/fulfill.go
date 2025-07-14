package fulfiller

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/chains"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// fulfillIntent attempts to fulfill a single intent
func (s *Fulfiller) fulfillIntent(ctx context.Context, intent models.Intent) error {
	s.mu.Lock()
	chainClient, exists := s.chainClients[intent.DestinationChain]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("destination chain configuration not found for: %d", intent.DestinationChain)
	}

	// Update gas price before transaction
	finalGasPrice := chainClient.GetCurrentGasPrice()
	if finalGasPrice == nil {
		s.logger.DebugWithChain(intent.DestinationChain, "Fetched gas price is nil")
		// Continue with default/previous gas price
	} else if finalGasPrice.Cmp(big.NewInt(0)) <= 0 {
		s.logger.DebugWithChain(intent.DestinationChain, "Fetched gas price is zero or negative: %s", finalGasPrice.String())
	} else {
		// Update metric (convert to gwei for readability)
		gasPriceGwei := new(big.Float).Quo(
			new(big.Float).SetInt(finalGasPrice),
			big.NewFloat(1e9), // 1 gwei = 10^9 wei
		)
		gweiFlt, _ := gasPriceGwei.Float64()
		metrics.GasPrice.WithLabelValues(fmt.Sprintf("%d", intent.DestinationChain)).Set(gweiFlt)
		s.logger.DebugWithChain(intent.DestinationChain, "Updated gas price: %.2f gwei", gweiFlt)
	}

	// Convert intent ID to bytes32
	intentID := common.HexToHash(intent.ID)

	// Convert amount to big.Int
	amount, ok := new(big.Int).SetString(intent.Amount, 10)
	if !ok {
		return fmt.Errorf("invalid amount: %s", intent.Amount)
	}

	// convert for BSC unit difference
	// TODO: use the token decimal attribute to convert amounts correctly
	if intent.SourceChain == 56 {
		amount = new(big.Int).Div(amount, big.NewInt(1000000000000))
	} else if intent.DestinationChain == 56 {
		amount = new(big.Int).Mul(amount, big.NewInt(1000000000000))
	}

	s.logger.InfoWithChain(intent.DestinationChain, "Fulfilling intent %s with amount %s", intent.ID, amount.String())

	// Convert addresses
	receiver := common.HexToAddress(intent.Recipient)

	// Get the Intent contract address
	intentAddress := common.HexToAddress(chainClient.IntentAddress)

	// Get the token type from token address
	tokenType := chains.GetTokenType(intent.Token)
	if tokenType == "" {
		return fmt.Errorf("token type not specified in intent: %s", intent.ID)
	}

	tokenAddress := chains.GetTokenEthAddress(intent.DestinationChain, tokenType)
	s.logger.DebugWithChain(intent.DestinationChain, "Using token %s address %s",
		tokenType, tokenAddress.Hex(),
	)

	// First, approve the token transfer
	// We need to approve the Intent contract to spend our tokens
	s.logger.DebugWithChain(intent.DestinationChain, "Checking token allowance for intent %s (token: %s, spender: %s)",
		intent.ID, tokenAddress.Hex(), intentAddress.Hex(),
	)

	erc20ABI, err := abi.JSON(strings.NewReader(contracts.ERC20ABI))
	if err != nil {
		return fmt.Errorf("failed to parse ERC20 ABI: %v", err)
	}

	// Create ERC20 contract binding
	erc20Contract := bind.NewBoundContract(
		tokenAddress,
		erc20ABI,
		chainClient.Client,
		chainClient.Client,
		chainClient.Client,
	)

	// Apply current gas price to transactor
	s.mu.Lock()
	txOpts := *chainClient.Auth
	s.mu.Unlock()

	// Check if approval is needed
	needsApproval := true

	// Check current allowance first
	callOpts := &bind.CallOpts{Context: ctx}

	// Use method call to get allowance
	var out []interface{}
	err = erc20Contract.Call(callOpts, &out, "allowance", txOpts.From, intentAddress)
	if err != nil {
		s.logger.DebugWithChain(
			intent.DestinationChain,
			"Failed to check allowance for intent %s: %v",
			intent.ID,
			err,
		)
		// Continue with approval (default behavior)
	} else if len(out) > 0 {
		if allowance, ok := out[0].(*big.Int); ok && allowance != nil {
			s.logger.DebugWithChain(intent.DestinationChain, "Current allowance for intent %s: %s (needed: %s)",
				intent.ID, allowance.String(), amount.String())
			if allowance.Cmp(amount) >= 0 {
				s.logger.DebugWithChain(intent.DestinationChain, "Existing allowance is sufficient for intent %s, skipping approval",
					intent.ID)
				needsApproval = false
			}
		}
	}

	// Proceed with approval if needed
	if needsApproval {
		s.logger.InfoWithChain(intent.DestinationChain, "Initiating token approval for intent %s (token: %s, spender: %s)",
			intent.ID, tokenAddress.Hex(), intentAddress.Hex())

		// Use max uint256 value for unlimited approval to avoid future approval transactions
		maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

		// Send the approve transaction with unlimited amount
		approveTx, err := erc20Contract.Transact(&txOpts, "approve", intentAddress, maxUint256)
		if err != nil {
			s.logger.ErrorWithChain(intent.DestinationChain, "Failed to create approval transaction for intent %s: %v", intent.ID, err)
			return fmt.Errorf("failed to approve token transfer: %v", err)
		}

		s.logger.InfoWithChain(intent.DestinationChain, "Approval transaction sent for intent %s: %s", intent.ID, approveTx.Hash().Hex())

		// Wait for the approve transaction to be mined
		approveReceipt, err := bind.WaitMined(ctx, chainClient.Client, approveTx)
		if err != nil {
			s.logger.ErrorWithChain(intent.DestinationChain, "Failed to mine approval transaction for intent %s: %v", intent.ID, err)
			return fmt.Errorf("failed to wait for approve transaction: %v", err)
		}

		if approveReceipt.Status == 0 {
			s.logger.ErrorWithChain(intent.DestinationChain, "Approval transaction failed for intent %s: %s", intent.ID, approveTx.Hash().Hex())
			return fmt.Errorf("approve transaction failed")
		}

		s.logger.InfoWithChain(intent.DestinationChain, "Approval successful for intent %s: %s (gas used: %d)",
			intent.ID, approveTx.Hash().Hex(), approveReceipt.GasUsed)
	}

	// Now call the contract's fulfill function with current gas price
	s.logger.NoticeWithChain(intent.DestinationChain, "Initiating fulfillment for intent %s (token: %s, amount: %s, receiver: %s)",
		intent.ID, tokenAddress.Hex(), amount.String(), receiver.Hex())

	tx, err := chainClient.IntentContract.Fulfill(&txOpts, intentID, tokenAddress, amount, receiver)
	if err != nil {
		s.logger.ErrorWithChain(intent.DestinationChain, "Failed to create fulfillment transaction for intent %s: %v", intent.ID, err)
		return fmt.Errorf("failed to fulfill intent on %d: %v", intent.DestinationChain, err)
	}

	s.logger.InfoWithChain(intent.DestinationChain, "Fulfillment transaction created for intent %s: %s", intent.ID, tx.Hash().Hex())

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, chainClient.Client, tx)
	if err != nil {
		s.logger.ErrorWithChain(intent.DestinationChain, "Failed to wait for transaction on intent %s: %v", intent.ID, err)
		return fmt.Errorf("failed to wait for transaction on %d: %v", intent.DestinationChain, err)
	}

	if receipt.Status == 0 {
		s.logger.ErrorWithChain(intent.DestinationChain, "Fulfillment transaction failed for intent %s: %s", intent.ID, tx.Hash().Hex())
		return fmt.Errorf("transaction failed on %d", intent.DestinationChain)
	}

	s.logger.NoticeWithChain(intent.DestinationChain, "Fulfillment transaction successful for intent %s: %s", intent.ID, tx.Hash().Hex())
	return nil
}
