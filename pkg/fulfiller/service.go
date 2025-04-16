package fulfiller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/config"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/health"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/metrics"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/models"
)

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
	tokenAddresses  map[int]common.Address
	workers         int
	pendingJobs     chan models.Intent
	retryJobs       chan models.RetryJob
	wg              sync.WaitGroup
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
}

// NewService creates a new fulfiller service
func NewService(cfg *config.Config) (*Service, error) {
	// Connect to blockchain clients
	for _, chainConfig := range cfg.Chains {
		if err := chainConfig.Connect(cfg.PrivateKey); err != nil {
			return nil, fmt.Errorf("failed to connect to chain %d: %v", chainConfig.ChainID, err)
		}
	}

	// Initialize token addresses map
	tokenAddresses := make(map[int]common.Address)

	// Set token addresses for each chain from environment variables
	initializeTokenAddresses(tokenAddresses)

	// Initialize circuit breakers
	circuitBreakers := make(map[int]*circuitbreaker.CircuitBreaker)
	for chainID := range cfg.Chains {
		circuitBreakers[chainID] = circuitbreaker.NewCircuitBreaker(
			cfg.CircuitBreaker.Enabled,
			cfg.CircuitBreaker.Threshold,
			cfg.CircuitBreaker.WindowDuration,
			cfg.CircuitBreaker.ResetTimeout,
		)
	}

	return &Service{
		config:          cfg,
		httpClient:      createHTTPClient(),
		tokenAddresses:  tokenAddresses,
		workers:         cfg.WorkerCount,
		pendingJobs:     make(chan models.Intent, 100),   // Buffer for pending intents
		retryJobs:       make(chan models.RetryJob, 100), // Buffer for retry jobs
		circuitBreakers: circuitBreakers,
	}, nil
}

// Start begins the fulfiller service
func (s *Service) Start(ctx context.Context) {
	// Start health monitoring server
	healthServer := health.NewServer(s.config.MetricsPort, s.config.Chains, s.circuitBreakers)
	go healthServer.Start()

	// Start worker pool
	log.Printf("Starting %d worker goroutines", s.workers)
	for i := 0; i < s.workers; i++ {
		go s.worker(ctx, i)
	}

	// Start retry handler
	go s.retryHandler(ctx)

	log.Printf("Starting Fulfiller Service with polling interval %v", s.config.PollingInterval)
	ticker := time.NewTicker(s.config.PollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, shutting down service")
			close(s.pendingJobs)
			close(s.retryJobs)
			s.wg.Wait() // Wait for all workers to finish
			return
		case <-ticker.C:
			intents, err := s.fetchPendingIntents()
			if err != nil {
				log.Printf("Error fetching intents: %v", err)
				continue
			}
			log.Printf("Found %d pending intents", len(intents))

			viableIntents := s.filterViableIntents(intents)
			log.Printf("Found %d viable intents for processing", len(viableIntents))

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Try to unmarshal into our wrapper struct first
	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		// If that fails, try directly as an array
		var intents []models.Intent
		if err := json.Unmarshal(bodyBytes, &intents); err != nil {
			return nil, fmt.Errorf("failed to decode intents: %v", err)
		}
		return intents, nil
	}

	// Handle paginated response with no data
	if apiResp.TotalCount == 0 {
		log.Printf("No pending intents found (page %d/%d, total count: %d)",
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
					log.Printf("Found intents in field: %s", key)
					break
				}
			}
		}

		if len(intents) == 0 {
			// This is a normal case when there are no pending intents
			log.Printf("No pending intents found in API response")
			return []models.Intent{}, nil
		}
	}
	return intents, nil
}

// filterViableIntents filters intents that are viable for fulfillment
func (s *Service) filterViableIntents(intents []models.Intent) []models.Intent {
	viableIntents := []models.Intent{}
	for _, intent := range intents {
		// check if source chain == destination chain
		if intent.SourceChain == intent.DestinationChain {
			log.Printf("Skipping intent %s: Source and destination chains are the same: %d",
				intent.ID, intent.SourceChain)
			continue
		}

		fee, success := new(big.Int).SetString(intent.IntentFee, 10)
		if !success {
			log.Printf("Skipping intent %s: Error parsing intent fee: invalid format", intent.ID)
			continue
		}
		if fee.Cmp(big.NewInt(0)) <= 0 {
			log.Printf("Skipping intent %s: Fee is zero or negative", intent.ID)
			continue
		}

		// Check if fee meets minimum requirement for the chain
		s.mu.Lock()
		destinationChainConfig, destinationExists := s.config.Chains[intent.DestinationChain]
		s.mu.Unlock()

		if !destinationExists {
			log.Printf("Skipping intent %s: Chain configuration not found for %d",
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
			log.Printf("Skipping intent %s: Fee %s below minimum %s for chain %d",
				intent.ID, fee.String(), destinationChainConfig.MinFee.String(), intent.DestinationChain)
			continue
		}

		viableIntents = append(viableIntents, intent)
	}
	return viableIntents
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
func initializeTokenAddresses(tokenAddresses map[int]common.Address) {
	if baseUSDC := getEnvAddr("BASE_USDC_ADDRESS"); baseUSDC != (common.Address{}) {
		tokenAddresses[8453] = baseUSDC
	}

	if arbitrumUSDC := getEnvAddr("ARBITRUM_USDC_ADDRESS"); arbitrumUSDC != (common.Address{}) {
		tokenAddresses[42161] = arbitrumUSDC
	}

	if polygonUSDC := getEnvAddr("POLYGON_USDC_ADDRESS"); polygonUSDC != (common.Address{}) {
		tokenAddresses[137] = polygonUSDC
	}

	if ethereumUSDC := getEnvAddr("ETHEREUM_USDC_ADDRESS"); ethereumUSDC != (common.Address{}) {
		tokenAddresses[1] = ethereumUSDC
	}

	if avalancheUSDC := getEnvAddr("AVALANCHE_USDC_ADDRESS"); avalancheUSDC != (common.Address{}) {
		tokenAddresses[43114] = avalancheUSDC
	}

	if bscUSDC := getEnvAddr("BSC_USDC_ADDRESS"); bscUSDC != (common.Address{}) {
		tokenAddresses[56] = bscUSDC
	}
}

// Helper to get address from environment
func getEnvAddr(key string) common.Address {
	if val := getEnv(key); val != "" {
		return common.HexToAddress(val)
	}
	return common.Address{}
}

// Helper to get environment variable
func getEnv(key string) string {
	return os.Getenv(key)
}
