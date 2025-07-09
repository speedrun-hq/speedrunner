package fulfiller

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/logger"

	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrunner/pkg/config"
	"github.com/speedrun-hq/speedrunner/pkg/health"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// TokenType represents the type of token
type TokenType string

const (
	// TokenTypeUSDC represents USDC token
	TokenTypeUSDC TokenType = "USDC"
	// TokenTypeUSDT represents USDT token
	TokenTypeUSDT TokenType = "USDT"
)

// Token represents a token with its address and metadata
type Token struct {
	Address common.Address
	Symbol  string
	Type    TokenType
}

// APIResponse represents the structure of the API response
type APIResponse struct {
	Intents    []models.Intent `json:"intents,omitempty"`
	Data       []models.Intent `json:"data,omitempty"`    // Some APIs use "data" as the key
	Results    []models.Intent `json:"results,omitempty"` // Some APIs use "results" as the key
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalCount int             `json:"total_count"`
	TotalPages int             `json:"total_pages"`
}

// Service handles the intent fulfillment process
type Service struct {
	config          *config.Config
	apiClient       *APIClient
	intentFilter    *IntentFilter
	tokenManager    *TokenManager
	metricsManager  *MetricsManager
	retryManager    *RetryManager
	mu              sync.Mutex
	workers         int
	pendingJobs     chan models.Intent
	wg              sync.WaitGroup
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
	nonceManager    *NonceManager
	logger          logger.Logger
}

// NewService creates a new fulfiller service
func NewService(cfg *config.Config) (*Service, error) {
	stdLogger := logger.NewStdLogger(cfg.LoggerConfig.Coloring, cfg.LoggerConfig.Level)

	// Connect to blockchain clients
	for _, chainConfig := range cfg.Chains {
		if err := chainConfig.Connect(cfg.PrivateKey); err != nil {
			return nil, fmt.Errorf("failed to connect to chain %d: %v", chainConfig.ChainID, err)
		}
	}

	// Initialize tokens map
	tokens := make(map[int]map[TokenType]Token)
	tokenAddressMap := make(map[common.Address]TokenType)

	// Initialize token map for each chain
	for chainID := range cfg.Chains {
		tokens[chainID] = make(map[TokenType]Token)
	}

	// Set token addresses for each chain from environment variables
	initializeTokens(tokens, tokenAddressMap)

	// Initialize circuit breakers
	circuitBreakers := make(map[int]*circuitbreaker.CircuitBreaker)
	for chainID := range cfg.Chains {
		circuitBreakers[chainID] = circuitbreaker.NewCircuitBreaker(
			cfg.CircuitBreaker.Enabled,
			cfg.CircuitBreaker.Threshold,
			cfg.CircuitBreaker.WindowDuration,
			cfg.CircuitBreaker.ResetTimeout,
			stdLogger,
		)
	}

	// Initialize new nonce manager
	nonceManager := NewNonceManager()

	// Initialize token manager
	tokenManager := NewTokenManager(stdLogger)
	// Set the tokens since the token manager doesn't have access to the config
	tokenManager.SetTokens(tokens, tokenAddressMap)

	return &Service{
		config:          cfg,
		apiClient:       NewAPIClient(cfg.APIEndpoint, stdLogger),
		intentFilter:    NewIntentFilter(cfg, tokenManager, stdLogger),
		tokenManager:    tokenManager,
		metricsManager:  NewMetricsManager(cfg, stdLogger),
		retryManager:    NewRetryManager(stdLogger),
		workers:         cfg.WorkerCount,
		pendingJobs:     make(chan models.Intent, 100), // Buffer for pending intents
		circuitBreakers: circuitBreakers,
		nonceManager:    nonceManager,
		logger:          stdLogger,
	}, nil
}

// Start begins the fulfiller service
func (s *Service) Start(ctx context.Context) {
	// Start health monitoring server
	healthServer := health.NewServer(
		s.config.MetricsPort,
		s.config.Chains,
		s.circuitBreakers,
		s.logger,
	)
	go healthServer.Start()

	// Start worker pool
	s.logger.Notice("Starting worker pool with %d workers", s.workers)
	for i := 0; i < s.workers; i++ {
		go s.worker(ctx, i)
	}

	// Start retry handler
	go s.retryHandler(ctx)

	// Start metrics updater
	go s.startMetricsUpdater(ctx)

	s.logger.Info("Starting Fulfiller Service with polling interval %v", s.config.PollingInterval)
	ticker := time.NewTicker(s.config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Notice("Context cancelled, shutting down service")
			close(s.pendingJobs)
			s.wg.Wait() // Wait for all workers to finish
			return
		case <-ticker.C:
			intents, err := s.fetchPendingIntents()
			if err != nil {
				s.logger.Error("Error fetching intents: %v", err)
				continue
			}
			s.logger.Debug("Found %d pending intents", len(intents))

			viableIntents := s.filterViableIntents(intents)
			s.logger.Info("Found %d viable intents for processing", len(viableIntents))

			// Update metric for pending intents
			metrics.PendingIntents.Set(float64(len(viableIntents)))

			// Queue viable intents for processing
			for _, intent := range viableIntents {
				s.wg.Add(1)
				s.pendingJobs <- intent
			}
		}
	}
}

// fetchPendingIntents gets pending intents from the API
func (s *Service) fetchPendingIntents() ([]models.Intent, error) {
	return s.apiClient.FetchPendingIntents()
}

// filterViableIntents filters intents that are viable for fulfillment
func (s *Service) filterViableIntents(intents []models.Intent) []models.Intent {
	return s.intentFilter.FilterViableIntents(intents, s.circuitBreakers)
}

// retryHandler processes retry jobs
func (s *Service) retryHandler(ctx context.Context) {
	s.logger.Info("Retry handler started")
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Retry handler shutting down")
			return
		case <-time.After(1 * time.Second):
			s.retryManager.processRetryJobs()
		}
	}
}

// Helper function to create an HTTP client with timeouts
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// Helper function to initialize token addresses
func initializeTokens(tokens map[int]map[TokenType]Token, tokenAddressMap map[common.Address]TokenType) {
	// Define chains
	// TODO: centralize in config
	chainList := []int{8453, 42161, 137, 1, 43114, 56, 7000}

	// Define token types
	tokenTypes := []struct {
		tokenType TokenType
		symbol    string
	}{
		{TokenTypeUSDC, "USDC"},
		{TokenTypeUSDT, "USDT"},
	}

	// Initialize tokens for each chain and token type
	for _, chainID := range chainList {
		// Ensure the chain map exists
		if _, exists := tokens[chainID]; !exists {
			tokens[chainID] = make(map[TokenType]Token)
		}

		for _, tokenInfo := range tokenTypes {
			var tokenAddrStr string

			switch tokenInfo.tokenType {
			case TokenTypeUSDC:
				tokenAddrStr = config.GetUSDCAddress(chainID)
			case TokenTypeUSDT:
				tokenAddrStr = config.GetUSDTAddress(chainID)
			default:
				tokenAddrStr = ""
			}
			if tokenAddrStr == "" {
				continue
			}
			tokenAddr := common.HexToAddress(tokenAddrStr)
			if tokenAddr == (common.Address{}) {
				continue
			}

			// Initialize token
			tokens[chainID][tokenInfo.tokenType] = Token{
				Address: tokenAddr,
				Symbol:  tokenInfo.symbol,
				Type:    tokenInfo.tokenType,
			}

			// Add to address map for quick lookups
			tokenAddressMap[tokenAddr] = tokenInfo.tokenType
		}
	}
}

// updateMetrics updates all metrics
func (s *Service) updateMetrics() {
	s.logger.Debug("Updating metrics...")

	// Update gas prices for each chain
	for chainID, chainConfig := range s.config.Chains {
		gasPrice, err := chainConfig.UpdateGasPrice(context.Background())
		if err != nil {
			s.logger.DebugWithChain(chainID, "Failed to update gas price: %v", err)
			continue
		}

		// Convert to gwei for readability
		gasPriceGwei := new(big.Float).Quo(
			new(big.Float).SetInt(gasPrice),
			big.NewFloat(1e9), // 1 gwei = 10^9 wei
		)
		gweiFlt, _ := gasPriceGwei.Float64()
		metrics.GasPrice.WithLabelValues(strconv.Itoa(chainID)).Set(gweiFlt)

		s.logger.DebugWithChain(chainID, "Updated gas price: %.2f gwei", gweiFlt)
	}

	// Update token balance metrics
	for chainID, chainTokens := range s.tokenManager.GetAllTokens() {
		chainName := config.GetChainName(chainID)
		s.logger.DebugWithChain(chainID, "Processing token balances")

		for tokenType, token := range chainTokens {

			balance, err := s.intentFilter.getTokenBalance(chainID, token.Address)
			if err != nil {
				s.logger.DebugWithChain(chainID, "Error getting token balance for %s: %v", tokenType, err)
				continue
			}

			// Convert balance to float64 for metrics
			balanceFloat, _ := balance.Float64()
			metrics.TokenBalance.WithLabelValues(chainName, string(tokenType)).Set(balanceFloat)

			s.logger.DebugWithChain(chainID, "Token %s balance: %f", tokenType, balanceFloat)
		}
	}

	// Update retry queue size
	queueSize := len(s.retryManager.GetRetryJobsChannel())
	s.logger.Debug("Setting retry queue size metric: %d", queueSize)
	metrics.RetryQueueSize.Set(float64(queueSize))
}

// startMetricsUpdater starts a goroutine to update metrics periodically
func (s *Service) startMetricsUpdater(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateMetrics()
		}
	}
}
