package config

import "strings"

// chainNames maps chain IDs to their names
var chainNames = map[int]string{
	1:     "ETHEREUM",
	137:   "POLYGON",
	42161: "ARBITRUM",
	43114: "AVALANCHE",
	56:    "BSC",
	7000:  "ZETACHAIN",
	8453:  "BASE",
}

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

// GetChainName returns the name of the chain for a given chain ID
func GetChainName(chainID int) string {
	name, exists := chainNames[chainID]
	if !exists {
		return ""
	}
	return name
}

// GetUSDCAddress returns the USDC contract address for a given chain ID
func GetUSDCAddress(chainID int) string {
	address, exists := usdcAddresses[chainID]
	if !exists {
		return ""
	}
	return address
}

// GetUSDTAddress returns the USDT contract address for a given chain ID
func GetUSDTAddress(chainID int) string {
	address, exists := usdtAddresses[chainID]
	if !exists {
		return ""
	}
	return address
}

// GetTokenType returns from the address the name of the token (USDC or USDT)
// It walk through all addresses maps, compare with address converted to lowercase and returns the token type if found
// return an empty string if not found
func GetTokenType(address string) string {
	// convert address to lowercase for case-insensitive comparison
	address = strings.ToLower(address)

	for _, usdcAddress := range usdcAddresses {
		if strings.ToLower(usdcAddress) == address {
			return "USDC"
		}
	}

	for _, usdtAddress := range usdtAddresses {
		if strings.ToLower(usdtAddress) == address {
			return "USDT"
		}
	}

	return ""
}
