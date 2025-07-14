package config

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
)

// Config holds the configuration for the fulfiller service
type Config struct {
	APIEndpoint      string
	PollingInterval  time.Duration
	FulfillerAddress string
	PrivateKey       string
	Chains           map[int]ChainConfig
	WorkerCount      int
	MetricsPort      string
	CircuitBreaker   CircuitBreakerConfig
	MaxRetries       int
	MaxGasPrice      *big.Int
	LoggerConfig     LoggerConfig
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled        bool
	Threshold      int
	WindowDuration time.Duration
	ResetTimeout   time.Duration
}

// LoggerConfig holds the configuration for logging
type LoggerConfig struct {
	Level    logger.Level
	Coloring bool
}

// ChainConfig holds the configuration for a specific blockchain
type ChainConfig struct {
	ChainID       int
	RPCURL        string
	IntentAddress string
	MinFee        string
}

// LoadConfig loads the configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables")
	}

	pollingInterval, err := GetEnvPollingInterval()
	if err != nil {
		return nil, err
	}

	workerCount, err := GetEnvWorkerCount()
	if err != nil {
		return nil, err
	}

	metricsPort, err := GetEnvMetricsPort()
	if err != nil {
		return nil, err
	}

	fulfillerAddress, err := GetEnvFulfillerAddress()
	if err != nil {
		return nil, err
	}

	cbEnabled, err := GetEnvCircuitBreakerEnabled()
	if err != nil {
		return nil, err
	}

	cbThreshold, err := GetEnvCircuitBreakerThreshold()
	if err != nil {
		return nil, err
	}

	cbWindow, err := GetEnvCircuitBreakerWindow()
	if err != nil {
		return nil, err
	}

	cbReset, err := GetEnvCircuitBreakerReset()
	if err != nil {
		return nil, err
	}

	maxRetries, err := GetEnvMaxRetries()
	if err != nil {
		return nil, err
	}

	maxGasPrice, err := GetEnvMaxGasPrice()
	if err != nil {
		return nil, err
	}

	apiEndpoint, err := GetEnvAPIEndpoint()
	if err != nil {
		return nil, err
	}

	logLever, err := GetEnvLogLevel()
	if err != nil {
		return nil, err
	}

	logColoring, err := GetEnvLogColoring()
	if err != nil {
		return nil, err
	}

	// Initialize chain configurations
	chainConfigs := make(map[int]ChainConfig)
	chainConfigList, err := GetEnvChainConfigs(mainnet)
	if err != nil {
		return nil, err
	}
	for _, chainConfig := range chainConfigList {
		chainConfigs[chainConfig.ChainID] = chainConfig
	}

	cfg := &Config{
		APIEndpoint:      apiEndpoint,
		PollingInterval:  pollingInterval,
		FulfillerAddress: fulfillerAddress,
		PrivateKey:       os.Getenv("PRIVATE_KEY"),
		Chains:           chainConfigs,
		WorkerCount:      workerCount,
		MetricsPort:      metricsPort,
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:        cbEnabled,
			Threshold:      cbThreshold,
			WindowDuration: cbWindow,
			ResetTimeout:   cbReset,
		},
		LoggerConfig: LoggerConfig{
			Level:    logLever,
			Coloring: logColoring,
		},
		MaxRetries:  maxRetries,
		MaxGasPrice: maxGasPrice,
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
