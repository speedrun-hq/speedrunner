package fulfiller

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
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
		log.Printf("Warning: Failed to update gas price: %v", err)
		// Continue with default/previous gas price
	} else {
		// Update metric (convert to gwei for readability)
		gasPriceGwei := new(big.Float).Quo(
			new(big.Float).SetInt(finalGasPrice),
			big.NewFloat(1e9), // 1 gwei = 10^9 wei
		)
		gweiFlt, _ := gasPriceGwei.Float64()
		metrics.GasPrice.WithLabelValues(fmt.Sprintf("%d", intent.DestinationChain)).Set(gweiFlt)
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

	// Get token address from the map
	s.mu.Lock()
	tokenAddress, exists := s.tokenAddresses[intent.DestinationChain]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("token address not configured for chain: %d", intent.DestinationChain)
	}

	log.Printf("Using token address %s for chain %d", tokenAddress.Hex(), intent.DestinationChain)

	// First, approve the token transfer
	// We need to approve the Intent contract to spend our tokens
	erc20ABI, err := abi.JSON(strings.NewReader(`[
		{
			"constant": true,
			"inputs": [
				{
					"name": "_owner",
					"type": "address"
				},
				{
					"name": "_spender",
					"type": "address"
				}
			],
			"name": "allowance",
			"outputs": [
				{
					"name": "",
					"type": "uint256"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [
				{
					"name": "_spender",
					"type": "address"
				},
				{
					"name": "_value",
					"type": "uint256"
				}
			],
			"name": "approve",
			"outputs": [
				{
					"name": "",
					"type": "bool"
				}
			],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`))
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

	// Apply current gas price to transactor
	s.mu.Lock()
	txOpts := *chainConfig.Auth
	s.mu.Unlock()

	// Check if approval is needed
	needsApproval := true

	// Check current allowance first
	callOpts := &bind.CallOpts{Context: context.Background()}

	// Use method call to get allowance
	var out []interface{}
	err = erc20Contract.Call(callOpts, &out, "allowance", txOpts.From, intentAddress)
	if err != nil {
		log.Printf("Failed to check allowance: %v", err)
		// Continue with approval (default behavior)
	} else if len(out) > 0 {
		if allowance, ok := out[0].(*big.Int); ok && allowance != nil && allowance.Cmp(amount) >= 0 {
			log.Printf("Existing allowance (%s) is sufficient for amount (%s), skipping approval",
				allowance.String(), amount.String())
			needsApproval = false
		}
	}

	// Proceed with approval if needed
	if needsApproval {
		// Use max uint256 value for unlimited approval to avoid future approval transactions
		maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

		log.Printf("Setting unlimited approval for token %s on chain %d", tokenAddress.Hex(), intent.DestinationChain)

		// Send the approve transaction with unlimited amount
		approveTx, err := erc20Contract.Transact(&txOpts, "approve", intentAddress, maxUint256)
		if err != nil {
			return fmt.Errorf("failed to approve token transfer: %v", err)
		}

		// Wait for the approve transaction to be mined
		approveReceipt, err := bind.WaitMined(context.Background(), chainConfig.Client, approveTx)
		if err != nil {
			return fmt.Errorf("failed to wait for approve transaction: %v", err)
		}

		if approveReceipt.Status == 0 {
			return fmt.Errorf("approve transaction failed")
		}

		// Log the gas used for the approval
		metrics.GasUsed.WithLabelValues(fmt.Sprintf("%d_approval", intent.DestinationChain)).Observe(float64(approveReceipt.GasUsed))

		log.Printf("Set unlimited token approval for intent %s on chain %d (gas used: %d)",
			intent.ID, intent.DestinationChain, approveReceipt.GasUsed)
	}

	// Now call the contract's fulfill function with current gas price
	tx, err := chainConfig.Contract.Fulfill(&txOpts, intentID, tokenAddress, amount, receiver)
	if err != nil {
		return fmt.Errorf("failed to fulfill intent on %d: %v", intent.DestinationChain, err)
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), chainConfig.Client, tx)
	if err != nil {
		return fmt.Errorf("failed to wait for transaction on %d: %v", intent.DestinationChain, err)
	}

	if receipt.Status == 0 {
		return fmt.Errorf("transaction failed on %d", intent.DestinationChain)
	}

	// Update gas used metric
	metrics.GasUsed.WithLabelValues(fmt.Sprintf("%d", intent.DestinationChain)).Observe(float64(receipt.GasUsed))

	log.Printf("Successfully fulfilled intent %s on %d with transaction %s (gas used: %d)",
		intent.ID, intent.DestinationChain, tx.Hash().Hex(), receipt.GasUsed)
	return nil
}
