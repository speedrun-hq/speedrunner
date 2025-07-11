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

// GetChainName returns the name of the chain for a given chain ID
func GetChainName(chainID int) string {
	name, exists := chainNames[chainID]
	if !exists {
		return ""
	}
	return name
}
