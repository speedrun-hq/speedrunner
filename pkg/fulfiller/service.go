package fulfiller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/chains"
	"github.com/speedrun-hq/speedrunner/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrunner/pkg/config"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
	"github.com/speedrun-hq/speedrunner/pkg/health"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// Token represents a token with its address and metadata
type Token struct {
	Address common.Address
	Symbol  string
	Type    chains.TokenType
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
	httpClient      *http.Client
	mu              sync.Mutex
	workers         int
	pendingJobs     chan models.Intent
	retryJobs       chan models.RetryJob
	wg              sync.WaitGroup
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
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

	return &Service{
		config:          cfg,
		httpClient:      createHTTPClient(),
		workers:         cfg.WorkerCount,
		pendingJobs:     make(chan models.Intent, 100),   // Buffer for pending intents
		retryJobs:       make(chan models.RetryJob, 100), // Buffer for retry jobs
		circuitBreakers: circuitBreakers,
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
			close(s.retryJobs)
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
	resp, err := s.httpClient.Get(s.config.APIEndpoint + "/api/v1/intents?status=pending")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending intents: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.logger.Error("Failed to close response body: %v", err)
		}
	}(resp.Body)

	// Read the response body regardless of status code
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Try to unmarshal into our wrapper struct first
	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		// If that fails, try directly as an array
		var intents []models.Intent
		if err := json.Unmarshal(bodyBytes, &intents); err != nil {
			return nil, fmt.Errorf("failed to decode intents: %v, body: %s", err, string(bodyBytes))
		}
		return intents, nil
	}

	// Handle paginated response with no data
	if apiResp.TotalCount == 0 {
		s.logger.Debug("No pending intents found (page %d/%d, total count: %d)",
			apiResp.Page, apiResp.TotalPages, apiResp.TotalCount)
		return []models.Intent{}, nil
	}

	// Get intents from whatever field is populated
	var intents []models.Intent
	if len(apiResp.Intents) > 0 {
		intents = apiResp.Intents
	} else if len(apiResp.Data) > 0 {
		intents = apiResp.Data
	} else if len(apiResp.Results) > 0 {
		intents = apiResp.Results
	} else {
		// Try one more thing - maybe it's in a top level array with a different name
		// Parse as generic map and look for any array field
		var genericResp map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &genericResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %v", err)
		}

		for key, value := range genericResp {
			if arrayValue, ok := value.([]interface{}); ok && len(arrayValue) > 0 {
				// Found an array, try to convert it to intents
				arrayJSON, err := json.Marshal(arrayValue)
				if err != nil {
					continue
				}
				if err := json.Unmarshal(arrayJSON, &intents); err == nil && len(intents) > 0 {
					s.logger.Debug("Found intents in field: %s", key)
					break
				}
			}
		}

		if len(intents) == 0 {
			// This is a normal case when there are no pending intents
			s.logger.Debug("No pending intents found in API response")
			return []models.Intent{}, nil
		}
	}
	return intents, nil
}

// filterViableIntents filters intents that are viable for fulfillment
func (s *Service) filterViableIntents(intents []models.Intent) []models.Intent {
	var viableIntents []models.Intent
	for _, intent := range intents {
		// Check circuit breaker status
		if breaker, exists := s.circuitBreakers[intent.DestinationChain]; exists {
			if breaker.IsOpen() {
				s.logger.Info("Skipping intent %s: Circuit breaker is open for chain %d",
					intent.ID, intent.DestinationChain)
				continue
			}
		}

		// Check if source chain == destination chain
		if intent.SourceChain == intent.DestinationChain {
			s.logger.Debug("Skipping intent %s: Source and destination chains are the same: %d",
				intent.ID, intent.SourceChain)
			continue
		}

		// Check if intent is more than 2 minutes old, only process recent intent
		// TODO: allow to configure this in config
		intentAge := time.Since(intent.CreatedAt)
		if intentAge > 2*time.Minute {
			s.logger.Debug("Skipping intent %s: Intent is too old (age: %s)", intent.ID, intentAge.String())
			continue
		}

		// Check token balance
		if !s.hasSufficientBalance(intent) {
			s.logger.Debug("Skipping intent %s: Insufficient token balance for chain %d",
				intent.ID, intent.DestinationChain)
			continue
		}

		fee, success := new(big.Int).SetString(intent.IntentFee, 10)
		if !success {
			s.logger.Debug("Skipping intent %s: Error parsing intent fee: invalid format", intent.ID)
			continue
		}
		if fee.Cmp(big.NewInt(0)) <= 0 {
			s.logger.Debug("Skipping intent %s: Fee is zero or negative", intent.ID)
			continue
		}

		// Check if fee meets minimum requirement for the chain
		s.mu.Lock()
		destinationChainConfig, destinationExists := s.config.Chains[intent.DestinationChain]
		s.mu.Unlock()

		if !destinationExists {
			s.logger.Debug("Skipping intent %s: Chain configuration not found for %d",
				intent.ID, intent.DestinationChain)
			continue
		}

		// convert fee for BSC unit difference
		if intent.SourceChain == 56 {
			fee = new(big.Int).Div(fee, big.NewInt(1000000000000))
		} else if intent.DestinationChain == 56 {
			fee = new(big.Int).Mul(fee, big.NewInt(1000000000000))
		}

		// Check if fee meets minimum requirement for the chain
		if destinationChainConfig.MinFee != nil && fee.Cmp(destinationChainConfig.MinFee) < 0 {
			s.logger.Debug("Skipping intent %s: Fee %s below minimum %s for chain %d",
				intent.ID, fee.String(), destinationChainConfig.MinFee.String(), intent.DestinationChain)
			continue
		}

		viableIntents = append(viableIntents, intent)
	}
	return viableIntents
}

// hasSufficientBalance checks if we have sufficient token balance for the intent
func (s *Service) hasSufficientBalance(intent models.Intent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get token type from address
	tokenType := chains.GetTokenType(intent.Token)
	if tokenType == "" {
		s.logger.DebugWithChain(intent.DestinationChain, "Unknown token type for address %s", intent.Token)
		return false
	}

	// Get token for the destination chain
	token := chains.GetTokenEthAddress(intent.DestinationChain, tokenType)
	if token == (common.Address{}) {
		s.logger.DebugWithChain(intent.DestinationChain, "Invalid token address for %s", tokenType)
		return false
	}

	// Get token balance
	balance, err := s.getTokenBalance(intent.DestinationChain, token)
	if err != nil {
		s.logger.DebugWithChain(intent.DestinationChain, "Error getting token balance: %v", err)
		return false
	}

	// Convert intent amount to big.Int
	amount, success := new(big.Int).SetString(intent.Amount, 10)
	if !success {
		s.logger.DebugWithChain(intent.DestinationChain, "Error parsing intent amount: %s", intent.Amount)
		return false
	}

	// convert amount for BSC unit difference
	if intent.SourceChain == 56 {
		amount = new(big.Int).Div(amount, big.NewInt(1000000000000))
	} else if intent.DestinationChain == 56 {
		amount = new(big.Int).Mul(amount, big.NewInt(1000000000000))
	}

	// Check if we have sufficient balance
	amountFloat := new(big.Float).SetInt(amount)
	return balance.Cmp(amountFloat) >= 0
}

// getTokenBalance gets the token balance for a given chain and token address
func (s *Service) getTokenBalance(chainID int, tokenAddress common.Address) (*big.Float, error) {
	chainConfig, exists := s.config.Chains[chainID]
	if !exists {
		return nil, fmt.Errorf("chain configuration not found for chain %d", chainID)
	}

	// Create ERC20 contract instance
	token, err := contracts.NewERC20(tokenAddress, chainConfig.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create ERC20 contract: %v", err)
	}

	// Get raw balance
	rawBalance, err := token.BalanceOf(nil, common.HexToAddress(s.config.FulfillerAddress))
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance: %v", err)
	}

	// Normalize balance by dividing by 10^decimals
	balanceFloat := new(big.Float).SetInt(rawBalance)

	return balanceFloat, nil
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

// retryHandler handles retrying failed jobs with exponential backoff
func (s *Service) retryHandler(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processRetryJobs()
		}
	}
}

// processRetryJobs processes jobs in the retry queue
func (s *Service) processRetryJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for {
		select {
		case job := <-s.retryJobs:
			if now.Before(job.NextAttempt) {
				// Put the job back in the queue
				s.retryJobs <- job
				// Update next retry metric
				metrics.NextRetryIn.Set(time.Until(job.NextAttempt).Seconds())
				return
			}

			// Check if we've exceeded max retries
			if job.RetryCount >= s.config.MaxRetries {
				s.logger.Debug("Max retries exceeded for intent %s: %s", job.Intent.ID, job.ErrorType)
				metrics.MaxRetriesReached.WithLabelValues(
					fmt.Sprintf("%d", job.Intent.DestinationChain),
					job.ErrorType,
				).Inc()
				continue
			}

			// Check circuit breaker
			if breaker, exists := s.circuitBreakers[job.Intent.DestinationChain]; exists && breaker.IsOpen() {
				// Put the job back in the queue
				s.retryJobs <- job
				metrics.RetriesSkipped.WithLabelValues(
					fmt.Sprintf("%d", job.Intent.DestinationChain),
					"circuit_breaker_open",
				).Inc()
				return
			}

			// Check gas price
			if !s.isGasPriceAcceptable(job.Intent.DestinationChain) {
				// Put the job back in the queue
				s.retryJobs <- job
				metrics.RetriesSkipped.WithLabelValues(
					fmt.Sprintf("%d", job.Intent.DestinationChain),
					"gas_price_too_high",
				).Inc()
				return
			}

			// Process the job
			s.wg.Add(1)
			s.pendingJobs <- job.Intent
			metrics.RetriesExecuted.WithLabelValues(
				fmt.Sprintf("%d", job.Intent.DestinationChain),
				job.ErrorType,
			).Inc()
		default:
			return
		}
	}
}

// isGasPriceAcceptable checks if the current gas price is acceptable for the chain
func (s *Service) isGasPriceAcceptable(chainID int) bool {
	chainConfig, exists := s.config.Chains[chainID]
	if !exists {
		return false
	}

	// Get current gas price
	gasPrice, err := chainConfig.Client.SuggestGasPrice(context.Background())
	if err != nil {

		s.logger.ErrorWithChain(chainID, "Error getting gas price: %v", err)
		return false
	}

	// Check if gas price is within acceptable range
	if chainConfig.MaxGasPrice != nil && gasPrice.Cmp(chainConfig.MaxGasPrice) > 0 {
		s.logger.ErrorWithChain(chainID, "Gas price too high: %s > %s", gasPrice.String(), chainConfig.MaxGasPrice.String())
		return false
	}

	return true
}

// updateMetrics updates Prometheus metrics
func (s *Service) updateMetrics() {
	s.logger.Debug("Starting metrics update...")

	// Update token balance metrics
	for _, chainID := range chains.ChainList {
		chainName := chains.GetChainName(chainID)
		s.logger.DebugWithChain(chainID, "Processing token balances")

		for _, tokenType := range chains.Tokenlist {

			tokenAddress := chains.GetTokenEthAddress(chainID, tokenType)
			if tokenAddress == (common.Address{}) {
				s.logger.DebugWithChain(chainID, "No token address found for %s", tokenType)
				continue
			}

			balance, err := s.getTokenBalance(chainID, tokenAddress)
			if err != nil {
				s.logger.DebugWithChain(chainID, "Error getting token balance for %s: %v", tokenType, err)
				continue
			}

			// Get token decimals for logging
			token, err := contracts.NewERC20(tokenAddress, s.config.Chains[chainID].Client)
			if err != nil {
				s.logger.DebugWithChain(chainID, "Error creating token contract for %s: %v", tokenType, err)
				continue
			}
			decimals, err := token.Decimals(&bind.CallOpts{})
			if err != nil {
				s.logger.DebugWithChain(chainID, "Error getting decimals for %s: %v", tokenType, err)
				continue
			}

			// Convert balance to float64 for Prometheus
			decimalsFloat := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
			balance.Quo(balance, decimalsFloat)
			balanceFloat64, _ := balance.Float64()

			metrics.TokenBalance.WithLabelValues(
				chainName,
				string(tokenType),
			).Set(balanceFloat64)
		}
	}

	// Update gas price metrics
	for chainID, chainConfig := range s.config.Chains {
		chainName := chains.GetChainName(chainID)
		if chainName == "" {
			chainName = "Unknown"
		}

		gasPrice, err := chainConfig.Client.SuggestGasPrice(context.Background())
		if err != nil {
			s.logger.DebugWithChain(chainID, "Error getting gas price: %v", err)
			continue
		}

		// Convert gas price to gwei for Prometheus
		gasPriceGwei := new(big.Float).Quo(
			new(big.Float).SetInt(gasPrice),
			new(big.Float).SetInt(big.NewInt(1e9)),
		)
		gasPriceFloat64, _ := gasPriceGwei.Float64()

		s.logger.DebugWithChain(chainID, "Setting gas price metric: %f gwei", gasPriceFloat64)
		metrics.GasPrice.WithLabelValues(
			chainName,
		).Set(gasPriceFloat64)
	}

	// Update retry queue size
	queueSize := len(s.retryJobs)
	s.logger.Debug("Setting retry queue size metric: %d", queueSize)
	metrics.RetryQueueSize.Set(float64(queueSize))

	s.logger.Debug("Metrics update completed")
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
