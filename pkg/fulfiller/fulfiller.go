package fulfiller

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/chainclient"
	"github.com/speedrun-hq/speedrunner/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrunner/pkg/config"
	"github.com/speedrun-hq/speedrunner/pkg/health"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
	"github.com/speedrun-hq/speedrunner/pkg/models"
	"github.com/speedrun-hq/speedrunner/pkg/srunclient"
)

// defaultChainMaxGas defines starting per-chain gas price caps in wei
var defaultChainMaxGas = map[int]string{
	1:     "150000000000", // Ethereum: 150 gwei
	137:   "100000000000", // Polygon: 100 gwei
	42161: "5000000000",   // Arbitrum: 5 gwei
	8453:  "5000000000",   // Base: 5 gwei
	56:    "20000000000",  // BSC: 20 gwei
	43114: "100000000000", // Avalanche: 100 gwei
	7000:  "50000000000",  // ZetaChain: 50 gwei (starting point)
}

// Fulfiller handles the intent fulfillment process
type Fulfiller struct {
	config          *config.Config
	srunClient      *srunclient.Client
	mu              sync.Mutex
	workers         int
	pendingJobs     chan models.Intent
	retryJobs       chan models.RetryJob
	wg              sync.WaitGroup
	chainClients    map[int]*chainclient.Client
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
	logger          logger.Logger
}

// NewFulfiller creates a new fulfiller service
func NewFulfiller(ctx context.Context, cfg *config.Config) (*Fulfiller, error) {
	stdLogger := logger.NewStdLogger(cfg.LoggerConfig.Coloring, cfg.LoggerConfig.Level)

	// Connect to blockchain clients
	chainClients := make(map[int]*chainclient.Client)
	for _, chainConfig := range cfg.Chains {
		chainClient, err := chainclient.New(
			ctx,
			chainConfig.ChainID,
			chainConfig.RPCURL,
			chainConfig.IntentAddress,
			chainConfig.MinFee,
			cfg.PrivateKey,
			stdLogger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create chain client for chain %d: %v", chainConfig.ChainID, err)
		}

		// Determine per-chain MaxGasPrice override: CHAIN_<ID>_MAX_GAS_PRICE
		var effectiveMaxGas *big.Int
		if val := os.Getenv(fmt.Sprintf("CHAIN_%d_MAX_GAS_PRICE", chainConfig.ChainID)); val != "" {
			if parsed, ok := new(big.Int).SetString(val, 10); ok {
				effectiveMaxGas = parsed
			} else {
				stdLogger.ErrorWithChain(chainConfig.ChainID, "Invalid CHAIN_%d_MAX_GAS_PRICE '%s', falling back to global", chainConfig.ChainID, val)
			}
		}
		if effectiveMaxGas == nil {
			// Check baked-in defaults by chain
			if def, ok := defaultChainMaxGas[chainConfig.ChainID]; ok {
				if parsed, ok2 := new(big.Int).SetString(def, 10); ok2 {
					effectiveMaxGas = parsed
				}
			}
		}
		if effectiveMaxGas == nil {
			effectiveMaxGas = cfg.MaxGasPrice
		}
		chainClient.MaxGasPrice = effectiveMaxGas

		chainClients[chainConfig.ChainID] = chainClient
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

	return &Fulfiller{
		config:          cfg,
		srunClient:      srunclient.New(cfg.APIEndpoint, stdLogger),
		workers:         cfg.WorkerCount,
		pendingJobs:     make(chan models.Intent, 100),   // Buffer for pending intents
		retryJobs:       make(chan models.RetryJob, 100), // Buffer for retry jobs
		chainClients:    chainClients,
		circuitBreakers: circuitBreakers,
		logger:          stdLogger,
	}, nil
}

// Start begins the fulfiller service
func (s *Fulfiller) Start(ctx context.Context) {
	// Start health monitoring server
	healthServer := health.NewServer(
		s.config.MetricsPort,
		s.chainClients,
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

	s.logger.Info("Starting Fulfiller Fulfiller with polling interval %v", s.config.PollingInterval)
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
			intents, err := s.srunClient.FetchPendingIntents()
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

// retryHandler handles retrying failed jobs with exponential backoff
func (s *Fulfiller) retryHandler(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processRetryJobs(ctx)
		}
	}
}

// processRetryJobs processes jobs in the retry queue
func (s *Fulfiller) processRetryJobs(ctx context.Context) {
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
			if !s.isGasPriceAcceptable(ctx, job.Intent.DestinationChain) {
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
func (s *Fulfiller) isGasPriceAcceptable(ctx context.Context, chainID int) bool {
	chainClient, exists := s.chainClients[chainID]
	if !exists {
		return false
	}

	// Get effective (multiplied) gas price without mutating state
	gasPrice, err := chainClient.EffectiveGasPrice(ctx)
	if err != nil {

		s.logger.ErrorWithChain(chainID, "Error getting gas price: %v", err)
		return false
	}

	// Check if gas price is within acceptable range after multiplier
	if !chainClient.IsWithinMax(gasPrice) {
		s.logger.ErrorWithChain(chainID, "Gas price too high: %s > %s (after multiplier)", gasPrice.String(), chainClient.MaxGasPrice.String())
		return false
	}

	return true
}
