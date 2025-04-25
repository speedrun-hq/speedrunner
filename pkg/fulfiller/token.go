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
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/blockchain"
)

// predefined approval amounts
var (
	ZeroApproval = big.NewInt(0)
	// MaxUint256 represents the maximum possible uint256 value (2^256 - 1)
	MaxUint256 = new(big.Int).Sub(new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil), big.NewInt(1))
	// threshold for deciding when to use infinite approval (if transaction amount > 30% of current allowance)
	ApprovalThreshold = big.NewFloat(0.3)
)

// ERC20ABI contains the ABI for ERC20 token functions needed for approvals
const ERC20ABI = `[
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
]`

// OptimizedTokenApproval handles token approvals with gas optimization strategy
// It returns true if a new approval was needed and executed successfully, false if no approval was needed
func (s *Service) OptimizedTokenApproval(
	ctx context.Context,
	chainConfig *blockchain.ChainConfig,
	chainID int,
	tokenType TokenType,
	spenderAddress common.Address,
	amount *big.Int,
) (bool, error) {
	// Get token from the token map
	s.mu.Lock()
	chainTokens, exists := s.tokens[chainID]
	s.mu.Unlock()

	if !exists {
		return false, fmt.Errorf("tokens not configured for chain: %d", chainID)
	}

	token, exists := chainTokens[tokenType]
	if !exists {
		return false, fmt.Errorf("token type %s not configured for chain: %d", tokenType, chainID)
	}

	tokenAddress := token.Address
	log.Printf("Processing token approval for %s (%s) at address %s", token.Symbol, tokenType, tokenAddress.Hex())

	// Create ERC20 contract binding
	erc20ABI, err := abi.JSON(strings.NewReader(ERC20ABI))
	if err != nil {
		return false, fmt.Errorf("failed to parse ERC20 ABI: %v", err)
	}

	// Create contract binding
	erc20Contract := bind.NewBoundContract(
		tokenAddress,
		erc20ABI,
		chainConfig.Client,
		chainConfig.Client,
		chainConfig.Client,
	)

	// Create call options
	callOpts := &bind.CallOpts{Context: ctx}

	// Check current allowance
	var out []interface{}
	err = erc20Contract.Call(callOpts, &out, "allowance", chainConfig.Auth.From, spenderAddress)
	if err != nil {
		return false, fmt.Errorf("failed to check allowance: %v", err)
	}

	// Ensure we got an allowance value
	if len(out) == 0 || out[0] == nil {
		return false, fmt.Errorf("empty allowance response")
	}

	// Parse allowance
	currentAllowance, ok := out[0].(*big.Int)
	if !ok || currentAllowance == nil {
		return false, fmt.Errorf("invalid allowance format")
	}

	log.Printf("Current allowance for token %s (%s): %s", token.Symbol, tokenType, currentAllowance.String())

	// Check if current allowance is sufficient
	if currentAllowance.Cmp(amount) >= 0 {
		log.Printf("Existing allowance (%s) is sufficient for amount (%s), skipping approval",
			currentAllowance.String(), amount.String())
		return false, nil
	}

	// Determine optimal approval amount
	approvalAmount := determineApprovalAmount(amount, currentAllowance)
	log.Printf("Setting approval amount to %s for token %s (%s)", approvalAmount.String(), token.Symbol, tokenType)

	// Apply current gas price to transactor
	txOpts := *chainConfig.Auth

	// If current allowance is not zero and we're using a new amount, we need to reset first for some tokens
	// This is for tokens that don't allow changing allowance from non-zero to non-zero values
	if currentAllowance.Cmp(ZeroApproval) > 0 && approvalAmount.Cmp(currentAllowance) != 0 {
		// Check if we need to reset allowance first (only for non-standard ERC20 implementations)
		if s.shouldResetAllowance(tokenAddress) {
			log.Printf("Resetting allowance for token %s (%s) before setting new allowance", token.Symbol, tokenType)
			resetTx, err := erc20Contract.Transact(&txOpts, "approve", spenderAddress, ZeroApproval)
			if err != nil {
				return false, fmt.Errorf("failed to reset token allowance: %v", err)
			}

			resetReceipt, err := bind.WaitMined(ctx, chainConfig.Client, resetTx)
			if err != nil {
				return false, fmt.Errorf("failed to wait for reset approval transaction: %v", err)
			}

			if resetReceipt.Status == 0 {
				return false, fmt.Errorf("reset approve transaction failed")
			}

			log.Printf("Successfully reset approval for token %s (%s)", token.Symbol, tokenType)
		}
	}

	// Send the approve transaction with the optimized amount
	approveTx, err := erc20Contract.Transact(&txOpts, "approve", spenderAddress, approvalAmount)
	if err != nil {
		return false, fmt.Errorf("failed to approve token transfer: %v", err)
	}

	// Wait for the approve transaction to be mined
	approveReceipt, err := bind.WaitMined(ctx, chainConfig.Client, approveTx)
	if err != nil {
		return false, fmt.Errorf("failed to wait for approve transaction: %v", err)
	}

	if approveReceipt.Status == 0 {
		return false, fmt.Errorf("approve transaction failed")
	}

	log.Printf("Successfully approved token %s (%s) for spender %s with amount %s (gas used: %d)",
		token.Symbol, tokenType, spenderAddress.Hex(), approvalAmount.String(), approveReceipt.GasUsed)

	return true, nil
}

// determineApprovalAmount decides the optimal approval amount based on current state
func determineApprovalAmount(requiredAmount, currentAllowance *big.Int) *big.Int {
	// If allowance is zero, always use infinite approval
	if currentAllowance.Cmp(ZeroApproval) == 0 {
		return MaxUint256
	}

	// Calculate if the required amount is significant compared to current allowance
	if currentAllowance.Cmp(ZeroApproval) > 0 {
		requiredFloat := new(big.Float).SetInt(requiredAmount)
		allowanceFloat := new(big.Float).SetInt(currentAllowance)

		// ratio = requiredAmount / currentAllowance
		ratio := new(big.Float).Quo(requiredFloat, allowanceFloat)

		// If we need more than 30% of the remaining allowance, switch to infinite approval
		if ratio.Cmp(ApprovalThreshold) > 0 {
			return MaxUint256
		}
	}

	// Default: provide exact required amount
	return requiredAmount
}

// shouldResetAllowance determines if a token requires allowance reset before changing it
// Some non-compliant ERC20 tokens require allowance to be reset to 0 before changing from one non-zero value to another
func (s *Service) shouldResetAllowance(tokenAddress common.Address) bool {
	// This is a simplified implementation - in production you would want to maintain a list
	// of known tokens that require resets, or implement a detection mechanism

	// Example: convert address to lowercase string for comparison
	tokenAddrStr := strings.ToLower(tokenAddress.Hex())

	// List of known tokens that require reset before approval (example)
	tokensRequiringReset := map[string]bool{
		// Add specific token addresses here as needed
		// "0x1234567890123456789012345678901234567890": true,
	}

	return tokensRequiringReset[tokenAddrStr]
}
