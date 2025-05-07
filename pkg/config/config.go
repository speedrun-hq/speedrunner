package config

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/blockchain"
)

// Config holds the configuration for the fulfiller service
type Config struct {
	APIEndpoint      string
	PollingInterval  time.Duration
	FulfillerAddress string
	PrivateKey       string
	Chains           map[int]*blockchain.ChainConfig
	WorkerCount      int
	MetricsPort      string
	CircuitBreaker   CircuitBreakerConfig
	MaxRetries       int
	MaxGasPrice      *big.Int
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled        bool
	Threshold      int
	WindowDuration time.Duration
	ResetTimeout   time.Duration
}

// LoadConfig loads the configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables")
	}

	// Load polling interval
	pollingInterval, err := strconv.Atoi(os.Getenv("POLLING_INTERVAL"))
	if err != nil || pollingInterval <= 0 {
		pollingInterval = 5 // default value
	}

	// Load worker count
	workerCount, err := strconv.Atoi(os.Getenv("WORKER_COUNT"))
	if err != nil || workerCount <= 0 {
		workerCount = 5 // default value
	}

	// Load metrics port
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "8080" // default value
	}

	// Load Fulfiller Address
	fulfillerAddress := os.Getenv("FULFILLER_ADDRESS")
	if fulfillerAddress == "" {
		fulfillerAddress = "0x0000000000000000000000000000000000000000" // default value
	}

	// Load circuit breaker configuration
	cbEnabled, _ := strconv.ParseBool(os.Getenv("CIRCUIT_BREAKER_ENABLED"))
	cbThreshold, err := strconv.Atoi(os.Getenv("CIRCUIT_BREAKER_THRESHOLD"))
	if err != nil || cbThreshold <= 0 {
		cbThreshold = 5 // Default: trip after 5 failures
	}

	cbWindowStr := os.Getenv("CIRCUIT_BREAKER_WINDOW")
	cbWindow := 5 * time.Minute // Default: 5 minute window
	if cbWindowStr != "" {
		if parsedWindow, err := time.ParseDuration(cbWindowStr); err == nil {
			cbWindow = parsedWindow
		}
	}

	cbResetStr := os.Getenv("CIRCUIT_BREAKER_RESET")
	cbReset := 15 * time.Minute // Default: 15 minute reset timeout
	if cbResetStr != "" {
		if parsedReset, err := time.ParseDuration(cbResetStr); err == nil {
			cbReset = parsedReset
		}
	}

	// Load max retries
	maxRetries, err := strconv.Atoi(os.Getenv("MAX_RETRIES"))
	if err != nil || maxRetries <= 0 {
		maxRetries = 3 // default value
	}

	// Load max gas price
	maxGasPriceStr := os.Getenv("MAX_GAS_PRICE")
	var maxGasPrice *big.Int
	if maxGasPriceStr != "" {
		maxGasPrice = new(big.Int)
		maxGasPrice.SetString(maxGasPriceStr, 10)
	}

	// Initialize chain configurations
	chains := make(map[int]*blockchain.ChainConfig)

	// Define chain configurations
	chainConfigs := []struct {
		chainID      int
		rpcEnvVar    string
		intentEnvVar string
		minFeeEnvVar string
	}{
		{8453, "BASE_RPC_URL", "BASE_INTENT_ADDRESS", "BASE_MIN_FEE"},
		{42161, "ARBITRUM_RPC_URL", "ARBITRUM_INTENT_ADDRESS", "ARBITRUM_MIN_FEE"},
		{137, "POLYGON_RPC_URL", "POLYGON_INTENT_ADDRESS", "POLYGON_MIN_FEE"},
		{1, "ETHEREUM_RPC_URL", "ETHEREUM_INTENT_ADDRESS", "ETHEREUM_MIN_FEE"},
		{43114, "AVALANCHE_RPC_URL", "AVALANCHE_INTENT_ADDRESS", "AVALANCHE_MIN_FEE"},
		{56, "BSC_RPC_URL", "BSC_INTENT_ADDRESS", "BSC_MIN_FEE"},
		{7000, "ZETACHAIN_RPC_URL", "ZETACHAIN_INTENT_ADDRESS", "ZETACHAIN_MIN_FEE"},
	}

	// Set up each chain configuration
	for _, config := range chainConfigs {
		if rpcURL := os.Getenv(config.rpcEnvVar); rpcURL != "" {
			chains[config.chainID] = blockchain.NewChainConfig(
				config.chainID,
				rpcURL,
				os.Getenv(config.intentEnvVar),
				os.Getenv(config.minFeeEnvVar),
			)
		}
	}

	cfg := &Config{
		APIEndpoint:      os.Getenv("API_ENDPOINT"),
		PollingInterval:  time.Duration(pollingInterval) * time.Second,
		FulfillerAddress: fulfillerAddress,
		PrivateKey:       os.Getenv("PRIVATE_KEY"),
		Chains:           chains,
		WorkerCount:      workerCount,
		MetricsPort:      metricsPort,
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:        cbEnabled,
			Threshold:      cbThreshold,
			WindowDuration: cbWindow,
			ResetTimeout:   cbReset,
		},
		MaxRetries:  maxRetries,
		MaxGasPrice: maxGasPrice,
	}

	// Set default API endpoint if not provided
	if cfg.APIEndpoint == "" {
		cfg.APIEndpoint = "http://localhost:8080"
	}

	// Validate required environment variables
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateConfig validates the configuration
func validateConfig(cfg *Config) error {
	if cfg.PrivateKey == "" {
		return fmt.Errorf("PRIVATE_KEY environment variable is required")
	}
	if len(cfg.Chains) == 0 {
		return fmt.Errorf("at least one chain configuration is required")
	}
	for chainID, chainConfig := range cfg.Chains {
		if chainConfig.IntentAddress == "" {
			return fmt.Errorf("%d_INTENT_ADDRESS for chain %d is required", chainID, chainID)
		}
	}
	return nil
}
