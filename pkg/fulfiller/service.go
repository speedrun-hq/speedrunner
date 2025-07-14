package fulfiller

import (
	"context"
	"fmt"
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

// Service handles the intent fulfillment process
type Service struct {
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

// NewService creates a new fulfiller service
func NewService(ctx context.Context, cfg *config.Config) (*Service, error) {
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
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create chain client for chain %d: %v", chainConfig.ChainID, err)
		}

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

	return &Service{
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
func (s *Service) Start(ctx context.Context) {
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
func (s *Service) retryHandler(ctx context.Context) {
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
func (s *Service) processRetryJobs(ctx context.Context) {
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
func (s *Service) isGasPriceAcceptable(ctx context.Context, chainID int) bool {
	chainClient, exists := s.chainClients[chainID]
	if !exists {
		return false
	}

	// Get current gas price
	gasPrice, err := chainClient.Client.SuggestGasPrice(ctx)
	if err != nil {

		s.logger.ErrorWithChain(chainID, "Error getting gas price: %v", err)
		return false
	}

	// Check if gas price is within acceptable range
	if chainClient.MaxGasPrice != nil && gasPrice.Cmp(chainClient.MaxGasPrice) > 0 {
		s.logger.ErrorWithChain(chainID, "Gas price too high: %s > %s", gasPrice.String(), chainClient.MaxGasPrice.String())
		return false
	}

	return true
}
