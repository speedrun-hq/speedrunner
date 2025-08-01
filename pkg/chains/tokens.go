package chains

import (
	"errors"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// TokenType represents the type of token
type TokenType string

const (
	// TokenTypeUSDC represents USDC token
	TokenTypeUSDC TokenType = "USDC"
	// TokenTypeUSDT represents USDT token
	TokenTypeUSDT TokenType = "USDT"
)

// Tokenlist contains the supported token types
var Tokenlist = []TokenType{
	TokenTypeUSDC,
	TokenTypeUSDT,
}

// TODO: create a generic structure that lists all tokens and their attributes

// usdcAddresses maps chain IDs to USDC contract addresses
var usdcAddresses = map[int]string{
	1:     "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
	137:   "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
	42161: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
	43114: "0xb97ef9ef8734c71904d8002f8b6bc66dd9c48a6e",
	56:    "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d",
	7000:  "0x0cbe0dF132a6c6B4a2974Fa1b7Fb953CF0Cc798a",
	8453:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
}

// usdcDecimals maps chain IDs to USDC token decimals
var usdcDecimals = map[int]int{
	1:     6,  // Ethereum
	137:   6,  // Polygon
	42161: 6,  // Arbitrum
	43114: 6,  // Avalanche
	56:    18, // Binance Smart Chain
	7000:  6,  // ZetaChain
	8453:  6,  // Base
}

// usdtAddresses maps chain IDs to USDT contract addresses
var usdtAddresses = map[int]string{
	1:     "0xdAC17F958D2ee523a2206206994597C13D831ec7",
	137:   "0xc2132D05D31c914a87C6611C10748AEb04B58e8F",
	42161: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9",
	43114: "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7",
	56:    "0x55d398326f99059fF775485246999027B3197955",
	7000:  "0x7c8dDa80bbBE1254a7aACf3219EBe1481c6E01d7",
	8453:  "0x50c5725949A6F0c72E6C4a641F24049A917DB0Cb",
}

// usdtDecimals maps chain IDs to USDT token decimals
var usdtDecimals = map[int]int{
	1:     6,  // Ethereum
	137:   6,  // Polygon
	42161: 6,  // Arbitrum
	43114: 6,  // Avalanche
	56:    18, // Binance Smart Chain
	7000:  6,  // ZetaChain
	8453:  6,  // Base
}

func getUSDCAddress(chainID int) string {
	address, exists := usdcAddresses[chainID]
	if !exists {
		return ""
	}
	return address
}

func getUSDTAddress(chainID int) string {
	address, exists := usdtAddresses[chainID]
	if !exists {
		return ""
	}
	return address
}

// GetUSDCDecimals returns the number of decimals for USDC on a given chain
func GetUSDCDecimals(chainID int) int {
	decimals, exists := usdcDecimals[chainID]
	if !exists {
		return 6 // default to 6 decimals if not found
	}
	return decimals
}

// GetUSDTDecimals returns the number of decimals for USDT on a given chain
func GetUSDTDecimals(chainID int) int {
	decimals, exists := usdtDecimals[chainID]
	if !exists {
		return 6 // default to 6 decimals if not found
	}
	return decimals
}

// GetTokenType returns from the address the name of the token (USDC or USDT)
// return an empty string if not found
func GetTokenType(address string) TokenType {
	// convert address to lowercase for case-insensitive comparison
	address = strings.ToLower(address)

	for _, usdcAddress := range usdcAddresses {
		if strings.ToLower(usdcAddress) == address {
			return TokenTypeUSDC
		}
	}

	for _, usdtAddress := range usdtAddresses {
		if strings.ToLower(usdtAddress) == address {
			return TokenTypeUSDT
		}
	}

	return ""
}

// GetTokenAddress returns the contract address for a given token type and chain ID
func GetTokenAddress(chainID int, tokenType TokenType) string {
	switch tokenType {
	case TokenTypeUSDC:
		return getUSDCAddress(chainID)
	case TokenTypeUSDT:
		return getUSDTAddress(chainID)
	default:
		return ""
	}
}

// GetTokenEthAddress returns the Ethereum address for a given token type
func GetTokenEthAddress(chainID int, tokenType TokenType) common.Address {
	address := GetTokenAddress(chainID, tokenType)
	if address == "" {
		return common.Address{}
	}
	return common.HexToAddress(address)
}

// GetStandardizedAmount returns a float representing the standardized amount for a given token type
// 1000000 -> 1 USDC for Ethereum
func GetStandardizedAmount(baseAmount *big.Int, chainID int, tokenType TokenType) (float64, error) {
	if baseAmount == nil || baseAmount.Sign() <= 0 {
		return 0, errors.New("invalid base amount")
	}

	var decimals int
	switch tokenType {
	case TokenTypeUSDC:
		decimals = GetUSDCDecimals(chainID)
	case TokenTypeUSDT:
		decimals = GetUSDTDecimals(chainID)
	default:
		return 0, errors.New("unsupported token type")
	}

	// Convert to float64 with appropriate scaling
	scaledAmount := new(big.Float).Quo(new(big.Float).SetInt(baseAmount), big.NewFloat(math.Pow(10, float64(decimals))))

	result, _ := scaledAmount.Float64()
	return result, nil
}
