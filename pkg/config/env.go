package config

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/blockchain"
	"math/big"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	mainnet = "mainnet"
	testnet = "testnet"

	// DefaultNetwork is the default blockchain network to connect to
	DefaultNetwork = mainnet

	// DefaultPollingInterval defines the default polling interval in seconds
	DefaultPollingInterval = 5

	// DefaultWorkerCount defines the default number of workers to process intents
	DefaultWorkerCount = 5

	// DefaultMetricsPort defines the default port for the metrics server
	DefaultMetricsPort = "8080"

	// DefaultFulfillerAddress defines the default fulfiller address
	DefaultFulfillerAddress = "0x0000000000000000000000000000000000000000"

	// DefaultCircuitBreakerEnabled defines whether the circuit breaker is enabled
	DefaultCircuitBreakerEnabled = true

	// DefaultCircuitBreakerThreshold defines the number of failures before the circuit breaker trips
	DefaultCircuitBreakerThreshold = 5

	// DefaultCircuitBreakerWindow defines the time window for the circuit breaker
	DefaultCircuitBreakerWindow = 5

	// DefaultCircuitBreakerReset defines the reset timeout for the circuit breaker
	DefaultCircuitBreakerReset = 15

	// DefaultMaxRetries defines the maximum number of retries for failed operations
	DefaultMaxRetries = 10

	// DefaultMaxGasPrice defines the maximum gas price for transactions
	DefaultMaxGasPrice = "1000000000" // 1 Gwei

	// DefaultAPIEndpoint defines the default API endpoint for the Speedrun service
	DefaultAPIEndpoint = "https://api.speedrun.exchange"

	// Network specific values
	// Note: intent address values are not prefixed with "Default"
	// These are the values to use but can still be overridden by environment variables for debugging purposes

	// Min fee is the minimum fee in base value for each network for the intent to be picked up
	// For now these values represent base amount in USDC and USDT

	// Base

	BaseMainnetChainID       = 8453
	BaseMainnetIntentAddress = "0x999fce149FD078DCFaa2C681e060e00F528552f4"

	DefaultBaseRPCURL        = "https://mainnet.base.org"
	DefaultBaseMainnetMinFee = "100000"

	// Arbitrum

	ArbitrumMainnetChainID       = 42161
	ArbitrumMainnetIntentAddress = "0xD6B0E2a8D115cCA2823c5F80F8416644F3970dD2"

	DefaultArbitrumMainnetRPCURL = "https://arb1.arbitrum.io/rpc"
	DefaultArbitrumMainnetMinFee = "100000"

	// Polygon

	PolygonMainnetChainID       = 137
	PolygonMainnetIntentAddress = "0x4017717c550E4B6E61048D412a718D6A8078d264"

	DefaultPolygonMainnetRPCURL = "https://polygon-rpc.com"
	DefaultPolygonMainnetMinFee = "100000"

	// Ethereum

	EthereumMainnetChainID       = 1
	EthereumMainnetIntentAddress = "0x951AB2A5417a51eB5810aC44BC1fC716995C1CAB"

	DefaultEthereumMainnetRPCURL = "https://eth.llamarpc.com"
	DefaultEthereumMainnetMinFee = "1000000"

	// Avalanche

	AvalancheMainnetChainID       = 43114
	AvalancheMainnetIntentAddress = "0x9a22A7d337aF1801BEEcDBE7f4f04BbD09F9E5bb"

	DefaultAvalancheMainnetRPCURL = "https://avalanche-c-chain-rpc.publicnode.com"
	DefaultAvalancheMainnetMinFee = "100000"

	// Binance Smart Chain (BSC)

	BSCMainnetChainID       = 56
	BSCMainnetIntentAddress = "0x68282fa70a32E52711d437b6c5984B714Eec3ED0"

	DefaultBSCMainnetRPCURL = "https://bsc-dataseed.bnbchain.org"
	DefaultBSCMainnetMinFee = "400000000000000000"

	// ZetaChain

	ZetaChainMainnetChainID       = 7000
	ZetaChainMainnetIntentAddress = "0x986e2db1aF08688dD3C9311016026daD15969e09"

	DefaultZetaChainMainnetRPCURL = "https://zetachain-evm.blockpi.network/v1/rpc/public"
	DefaultZetaChainMainnetMinFee = "100000"
)

// GetEnvNetwork returns the configured network from environment variables or defaults to mainnet
func GetEnvNetwork() (string, error) {
	network := os.Getenv("NETWORK")
	if network == "" {
		network = DefaultNetwork
	}

	if network != mainnet && network != testnet {
		return "", fmt.Errorf("invalid NETWORK value: %s, must be 'mainnet' or 'testnet'", network)
	}

	return network, nil
}

// GetEnvPollingInterval returns the polling interval in seconds from environment variables
func GetEnvPollingInterval() (time.Duration, error) {
	pollingInterval := os.Getenv("POLLING_INTERVAL")
	if pollingInterval == "" {
		return time.Duration(DefaultPollingInterval) * time.Second, nil
	}

	// use atoi
	interval, err := strconv.Atoi(pollingInterval)
	if err != nil {
		return 0, fmt.Errorf("invalid POLLING_INTERVAL value: %s, must be an integer", pollingInterval)
	}
	if interval <= 0 {
		return 0, fmt.Errorf("POLLING_INTERVAL must be greater than 0")
	}
	return time.Duration(interval) * time.Second, nil
}

// GetEnvWorkerCount returns the number of workers from environment variables
func GetEnvWorkerCount() (int, error) {
	workerCount := os.Getenv("WORKER_COUNT")
	if workerCount == "" {
		return DefaultWorkerCount, nil
	}

	// use atoi
	count, err := strconv.Atoi(workerCount)
	if err != nil {
		return 0, fmt.Errorf("invalid WORKER_COUNT value: %s, must be an integer", workerCount)
	}
	if count <= 0 {
		return 0, fmt.Errorf("WORKER_COUNT must be greater than 0")
	}
	return count, nil
}

// GetEnvMetricsPort returns the metrics server port from environment variables
func GetEnvMetricsPort() (string, error) {
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		return DefaultMetricsPort, nil
	}

	// Validate port format
	if _, err := strconv.Atoi(metricsPort); err != nil {
		return "", fmt.Errorf("invalid METRICS_PORT value: %s, must be a valid integer", metricsPort)
	}
	return metricsPort, nil
}

// GetEnvFulfillerAddress returns the fulfiller address from environment variables
func GetEnvFulfillerAddress() (string, error) {
	fulfillerAddress := os.Getenv("FULFILLER_ADDRESS")
	if fulfillerAddress == "" {
		return DefaultFulfillerAddress, nil
	}

	// Validate Ethereum address format
	if !common.IsHexAddress(fulfillerAddress) {
		return "", fmt.Errorf("invalid FULFILLER_ADDRESS value: %s, must be a valid Ethereum address", fulfillerAddress)
	}
	return fulfillerAddress, nil
}

// GetEnvCircuitBreakerEnabled returns whether the circuit breaker is enabled from environment variables
func GetEnvCircuitBreakerEnabled() (bool, error) {
	enabled := os.Getenv("CIRCUIT_BREAKER_ENABLED")
	if enabled == "" {
		return DefaultCircuitBreakerEnabled, nil
	}

	if enabled == "true" {
		return true, nil
	} else if enabled == "false" {
		return false, nil
	}

	return false, fmt.Errorf("invalid CIRCUIT_BREAKER_ENABLED value: %s, must be 'true' or 'false'", enabled)
}

// GetEnvCircuitBreakerThreshold returns the circuit breaker threshold from environment variables
func GetEnvCircuitBreakerThreshold() (int, error) {
	threshold := os.Getenv("CIRCUIT_BREAKER_THRESHOLD")
	if threshold == "" {
		return DefaultCircuitBreakerThreshold, nil
	}

	thresholdInt, err := strconv.Atoi(threshold)
	if err != nil {
		return 0, fmt.Errorf("invalid CIRCUIT_BREAKER_THRESHOLD value: %s, must be an integer", threshold)
	}
	if thresholdInt <= 0 {
		return 0, fmt.Errorf("CIRCUIT_BREAKER_THRESHOLD must be greater than 0")
	}
	return thresholdInt, nil
}

// GetEnvCircuitBreakerWindow returns the circuit breaker window duration from environment variables
func GetEnvCircuitBreakerWindow() (time.Duration, error) {
	window := os.Getenv("CIRCUIT_BREAKER_WINDOW")
	if window == "" {
		return DefaultCircuitBreakerWindow * time.Second, nil
	}

	// Validate duration format
	parsed, err := time.ParseDuration(window)
	if err != nil {
		return 0, fmt.Errorf("invalid CIRCUIT_BREAKER_WINDOW value: %s, must be a valid duration string", window)
	}
	return parsed, nil
}

// GetEnvCircuitBreakerReset returns the circuit breaker reset timeout from environment variables
func GetEnvCircuitBreakerReset() (time.Duration, error) {
	reset := os.Getenv("CIRCUIT_BREAKER_RESET")
	if reset == "" {
		return DefaultCircuitBreakerReset * time.Second, nil
	}

	// Validate duration format
	parsed, err := time.ParseDuration(reset)
	if err != nil {
		return 0, fmt.Errorf("invalid CIRCUIT_BREAKER_RESET value: %s, must be a valid duration string", reset)
	}
	return parsed, nil
}

// GetEnvMaxRetries returns the maximum number of retries from environment variables
func GetEnvMaxRetries() (int, error) {
	maxRetries := os.Getenv("MAX_RETRIES")
	if maxRetries == "" {
		return DefaultMaxRetries, nil
	}

	maxRetriesInt, err := strconv.Atoi(maxRetries)
	if err != nil {
		return 0, fmt.Errorf("invalid MAX_RETRIES value: %s, must be an integer", maxRetries)
	}
	if maxRetriesInt < 0 {
		return 0, fmt.Errorf("MAX_RETRIES must be greater than or equal to 0")
	}
	return maxRetriesInt, nil
}

// GetEnvMaxGasPrice returns the maximum gas price from environment variables
func GetEnvMaxGasPrice() (*big.Int, error) {
	maxGasPrice := os.Getenv("MAX_GAS_PRICE")
	if maxGasPrice == "" {
		maxGasPrice = DefaultMaxGasPrice
	}

	maxGasPriceBig := new(big.Int)
	if _, ok := maxGasPriceBig.SetString(maxGasPrice, 10); !ok {
		return nil, fmt.Errorf("invalid MAX_GAS_PRICE value: %s, must be a valid integer string", maxGasPrice)
	}

	if maxGasPriceBig.Cmp(big.NewInt(0)) < 0 {
		return nil, fmt.Errorf("MAX_GAS_PRICE must be greater than or equal to 0")
	}
	return maxGasPriceBig, nil
}

// GetEnvAPIEndpoint returns the API endpoint from environment variables
func GetEnvAPIEndpoint() (string, error) {
	apiEndpoint := os.Getenv("API_ENDPOINT")
	if apiEndpoint == "" {
		return DefaultAPIEndpoint, nil
	}

	// Validate URL format
	if _, err := url.ParseRequestURI(apiEndpoint); err != nil {
		return "", fmt.Errorf("invalid API_ENDPOINT value: %s, must be a valid URL", apiEndpoint)
	}
	return apiEndpoint, nil
}

// GetEnvChainConfigs returns the chain configurations for all supported network based on the environment variables and network type
// TODO: refactor this to use a more generic approach for all chains
func GetEnvChainConfigs(network string) ([]*blockchain.ChainConfig, error) {
	// only mainnet currently supported
	if network != mainnet {
		return nil, fmt.Errorf("unsupported network: %s, only 'mainnet' is supported", network)
	}

	// base
	rpc := os.Getenv("BASE_RPC_URL")
	if rpc == "" {
		rpc = DefaultBaseRPCURL
	}
	intent := os.Getenv("BASE_INTENT_ADDRESS")
	if intent == "" {
		intent = BaseMainnetIntentAddress
	}
	minFee := os.Getenv("BASE_MIN_FEE")
	if minFee == "" {
		minFee = DefaultBaseMainnetMinFee
	}
	baseConfig := blockchain.NewChainConfig(
		BaseMainnetChainID,
		rpc,
		intent,
		minFee,
	)

	// arbitrum
	arbitrumRPC := os.Getenv("ARBITRUM_RPC_URL")
	if arbitrumRPC == "" {
		arbitrumRPC = DefaultArbitrumMainnetRPCURL
	}
	arbitrumIntent := os.Getenv("ARBITRUM_INTENT_ADDRESS")
	if arbitrumIntent == "" {
		arbitrumIntent = ArbitrumMainnetIntentAddress
	}
	arbitrumMinFee := os.Getenv("ARBITRUM_MIN_FEE")
	if arbitrumMinFee == "" {
		arbitrumMinFee = DefaultArbitrumMainnetMinFee
	}
	arbitrumConfig := blockchain.NewChainConfig(
		ArbitrumMainnetChainID,
		arbitrumRPC,
		arbitrumIntent,
		arbitrumMinFee,
	)

	// polygon

	polygonRPC := os.Getenv("POLYGON_RPC_URL")
	if polygonRPC == "" {
		polygonRPC = DefaultPolygonMainnetRPCURL
	}
	polygonIntent := os.Getenv("POLYGON_INTENT_ADDRESS")
	if polygonIntent == "" {
		polygonIntent = PolygonMainnetIntentAddress
	}
	polygonMinFee := os.Getenv("POLYGON_MIN_FEE")
	if polygonMinFee == "" {
		polygonMinFee = DefaultPolygonMainnetMinFee
	}
	polygonConfig := blockchain.NewChainConfig(
		PolygonMainnetChainID,
		polygonRPC,
		polygonIntent,
		polygonMinFee,
	)
	// ethereum
	ethereumRPC := os.Getenv("ETHEREUM_RPC_URL")
	if ethereumRPC == "" {
		ethereumRPC = DefaultEthereumMainnetRPCURL
	}
	ethereumIntent := os.Getenv("ETHEREUM_INTENT_ADDRESS")
	if ethereumIntent == "" {
		ethereumIntent = EthereumMainnetIntentAddress
	}
	ethereumMinFee := os.Getenv("ETHEREUM_MIN_FEE")
	if ethereumMinFee == "" {
		ethereumMinFee = DefaultEthereumMainnetMinFee
	}
	ethereumConfig := blockchain.NewChainConfig(
		EthereumMainnetChainID,
		ethereumRPC,
		ethereumIntent,
		ethereumMinFee,
	)

	// avalanche
	avalancheRPC := os.Getenv("AVALANCHE_RPC_URL")
	if avalancheRPC == "" {
		avalancheRPC = DefaultAvalancheMainnetRPCURL
	}
	avalancheIntent := os.Getenv("AVALANCHE_INTENT_ADDRESS")
	if avalancheIntent == "" {
		avalancheIntent = AvalancheMainnetIntentAddress
	}
	avalancheMinFee := os.Getenv("AVALANCHE_MIN_FEE")
	if avalancheMinFee == "" {
		avalancheMinFee = DefaultAvalancheMainnetMinFee
	}
	avalancheConfig := blockchain.NewChainConfig(
		AvalancheMainnetChainID,
		avalancheRPC,
		avalancheIntent,
		avalancheMinFee,
	)

	// bsc
	bscRPC := os.Getenv("BSC_RPC_URL")
	if bscRPC == "" {
		bscRPC = DefaultBSCMainnetRPCURL
	}
	bscIntent := os.Getenv("BSC_INTENT_ADDRESS")
	if bscIntent == "" {
		bscIntent = BSCMainnetIntentAddress
	}
	bscMinFee := os.Getenv("BSC_MIN_FEE")
	if bscMinFee == "" {
		bscMinFee = DefaultBSCMainnetMinFee
	}
	bscConfig := blockchain.NewChainConfig(
		BSCMainnetChainID,
		bscRPC,
		bscIntent,
		bscMinFee,
	)

	// zetachain
	zetachainRPC := os.Getenv("ZETACHAIN_RPC_URL")
	if zetachainRPC == "" {
		zetachainRPC = DefaultZetaChainMainnetRPCURL
	}
	zetachainIntent := os.Getenv("ZETACHAIN_INTENT_ADDRESS")
	if zetachainIntent == "" {
		zetachainIntent = ZetaChainMainnetIntentAddress
	}
	zetachainMinFee := os.Getenv("ZETACHAIN_MIN_FEE")
	if zetachainMinFee == "" {
		zetachainMinFee = DefaultZetaChainMainnetMinFee
	}
	zetachainConfig := blockchain.NewChainConfig(
		ZetaChainMainnetChainID,
		zetachainRPC,
		zetachainIntent,
		zetachainMinFee,
	)

	return []*blockchain.ChainConfig{
		baseConfig,
		arbitrumConfig,
		polygonConfig,
		ethereumConfig,
		avalancheConfig,
		bscConfig,
		zetachainConfig,
	}, nil
}
