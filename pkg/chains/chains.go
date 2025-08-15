package chains

// ChainList contains the list of supported chain IDs
var ChainList = []int{
	1,     // Ethereum
	137,   // Polygon
	42161, // Arbitrum
	43114, // Avalanche
	56,    // Binance Smart Chain
	7000,  // ZetaChain
	8453,  // Base
}

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

// WithdrawDefaultGasLimit is the default gas limit for withdrawal transactions per chain
// Exposed for use by other packages
var WithdrawDefaultGasLimit = map[int]uint64{
	1:     400000,  // Ethereum
	137:   400000,  // Polygon
	42161: 1000000, // Arbitrum
	43114: 400000,  // Avalanche
	56:    400000,  // Binance Smart Chain
	7000:  400000,  // ZetaChain
	8453:  400000,  // Base
}

// GetChainName returns the name of the chain for a given chain ID
func GetChainName(chainID int) string {
	name, exists := chainNames[chainID]
	if !exists {
		return ""
	}
	return name
}
