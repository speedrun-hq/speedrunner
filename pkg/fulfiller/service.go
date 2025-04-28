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
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/blockchain"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/config"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/health"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/metrics"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/models"
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

// AllowanceCacheKey is used as a key for the allowance cache
type AllowanceCacheKey struct {
	ChainID     int
	TokenAddr   common.Address
	OwnerAddr   common.Address
	SpenderAddr common.Address
}

// AllowanceCacheEntry represents a cached allowance entry
type AllowanceCacheEntry struct {
	Allowance  *big.Int
	UpdatedAt  time.Time
	Expiration time.Time
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
	tokens          map[int]map[TokenType]Token
	workers         int
	pendingJobs     chan models.Intent
	retryJobs       chan models.RetryJob
	wg              sync.WaitGroup
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
	allowanceCache  map[AllowanceCacheKey]AllowanceCacheEntry
	allowanceMu     sync.RWMutex             // Separate mutex for allowance cache to reduce contention
	nonceManager    *blockchain.NonceManager // Nonce manager for handling transaction nonces
}

// NewService creates a new fulfiller service
func NewService(cfg *config.Config) (*Service, error) {
	// Connect to blockchain clients
	for _, chainConfig := range cfg.Chains {
		if err := chainConfig.Connect(cfg.PrivateKey); err != nil {
			return nil, fmt.Errorf("failed to connect to chain %d: %v", chainConfig.ChainID, err)
		}
	}

	// Initialize tokens map
	tokens := make(map[int]map[TokenType]Token)

	// Initialize token map for each chain
	for chainID := range cfg.Chains {
		tokens[chainID] = make(map[TokenType]Token)
	}

	// Set token addresses for each chain from environment variables
	initializeTokens(tokens)

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

	// Initialize nonce manager
	nonceManager := blockchain.NewNonceManager()
	// Configure transaction timeout (optional, defaults to 5 minutes)
	nonceManager.SetTransactionTimeout(10 * time.Minute) // Longer timeout for blockchain transactions

	return &Service{
		config:          cfg,
		httpClient:      createHTTPClient(),
		tokens:          tokens,
		workers:         cfg.WorkerCount,
		pendingJobs:     make(chan models.Intent, 100),   // Buffer for pending intents
		retryJobs:       make(chan models.RetryJob, 100), // Buffer for retry jobs
		circuitBreakers: circuitBreakers,
		allowanceCache:  make(map[AllowanceCacheKey]AllowanceCacheEntry),
		nonceManager:    nonceManager,
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

	// Start transaction monitoring
	go func() {
		log.Println("Transaction monitor started")
		ticker := time.NewTicker(1 * time.Minute) // Check every minute
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Transaction monitor shutting down")
				return
			case <-ticker.C:
				s.TransactionMonitorTick(ctx)
			}
		}
	}()

	// Start transaction recovery job
	go s.transactionRecovery(ctx)

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
func initializeTokens(tokens map[int]map[TokenType]Token) {
	// Define chain configurations
	chainConfigs := []struct {
		chainID   int
		chainName string
	}{
		{8453, "BASE"},
		{42161, "ARBITRUM"},
		{137, "POLYGON"},
		{1, "ETHEREUM"},
		{43114, "AVALANCHE"},
		{56, "BSC"},
		{7000, "ZETACHAIN"},
	}

	// Define token types
	tokenTypes := []struct {
		tokenType TokenType
		symbol    string
	}{
		{TokenTypeUSDC, "USDC"},
		{TokenTypeUSDT, "USDT"},
	}

	// Initialize tokens for each chain and token type
	for _, chain := range chainConfigs {
		// Ensure the chain map exists
		if _, exists := tokens[chain.chainID]; !exists {
			tokens[chain.chainID] = make(map[TokenType]Token)
		}

		for _, tokenInfo := range tokenTypes {
			// Construct environment variable name
			envVarName := fmt.Sprintf("%s_%s_ADDRESS", chain.chainName, tokenInfo.symbol)

			// Get token address from environment
			tokenAddr := getEnvAddr(envVarName)

			// Skip if not configured
			if tokenAddr == (common.Address{}) {
				continue
			}

			// Initialize token
			tokens[chain.chainID][tokenInfo.tokenType] = Token{
				Address: tokenAddr,
				Symbol:  tokenInfo.symbol,
				Type:    tokenInfo.tokenType,
			}
		}
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

// queueForRetry adds an intent to the retry queue
func (s *Service) queueForRetry(intent models.Intent, errorType string, initialDelay time.Duration) {
	retryJob := models.RetryJob{
		Intent:      intent,
		RetryCount:  0,
		NextAttempt: time.Now().Add(initialDelay),
		ErrorType:   errorType,
	}
	s.retryJobs <- retryJob
}

// checkAndCacheAllowance checks if there's enough token allowance and caches the result
func (s *Service) checkAndCacheAllowance(ctx context.Context, chainConfig *blockchain.ChainConfig,
	tokenAddress, ownerAddress, spenderAddress common.Address, requiredAmount *big.Int,
) (bool, error) {
	// Create cache key
	cacheKey := AllowanceCacheKey{
		ChainID:     chainConfig.ChainID,
		TokenAddr:   tokenAddress,
		OwnerAddr:   ownerAddress,
		SpenderAddr: spenderAddress,
	}

	// Check cache first (with read lock)
	s.allowanceMu.RLock()
	entry, exists := s.allowanceCache[cacheKey]
	s.allowanceMu.RUnlock()

	now := time.Now()
	// If we have a cached entry that's still valid and sufficient
	if exists && now.Before(entry.Expiration) && entry.Allowance.Cmp(requiredAmount) >= 0 {
		log.Printf("Using cached allowance for token %s: %s",
			tokenAddress.Hex(), entry.Allowance.String())
		return true, nil
	}

	// Need to check allowance from blockchain
	abi, err := getERC20ABI()
	if err != nil {
		return false, fmt.Errorf("failed to get ERC20 ABI: %v", err)
	}

	// Create contract binding
	contract := bind.NewBoundContract(
		tokenAddress,
		abi,
		chainConfig.Client,
		chainConfig.Client,
		chainConfig.Client,
	)

	// Call allowance method
	callOpts := &bind.CallOpts{Context: ctx}
	var out []interface{}
	err = contract.Call(callOpts, &out, "allowance", ownerAddress, spenderAddress)
	if err != nil {
		return false, fmt.Errorf("failed to check allowance: %v", err)
	}

	// Process result
	if len(out) == 0 || out[0] == nil {
		return false, fmt.Errorf("empty result from allowance call")
	}

	allowance, ok := out[0].(*big.Int)
	if !ok || allowance == nil {
		return false, fmt.Errorf("invalid allowance result type")
	}

	// Cache the result (with write lock)
	s.allowanceMu.Lock()
	s.allowanceCache[cacheKey] = AllowanceCacheEntry{
		Allowance:  allowance,
		UpdatedAt:  now,
		Expiration: now.Add(10 * time.Minute), // Cache for 10 minutes
	}
	s.allowanceMu.Unlock()

	// Return whether allowance is sufficient
	return allowance.Cmp(requiredAmount) >= 0, nil
}

// Helper function to get the ERC20 ABI
func getERC20ABI() (abi.ABI, error) {
	return abi.JSON(strings.NewReader(`[
		{
			"constant": true,
			"inputs": [
				{
					"name": "_owner",
					"type": "address"
				},
				{
					"name": "_spender",
					"type": "address"
				}
			],
			"name": "allowance",
			"outputs": [
				{
					"name": "",
					"type": "uint256"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": false,
			"inputs": [
				{
					"name": "_spender",
					"type": "address"
				},
				{
					"name": "_value",
					"type": "uint256"
				}
			],
			"name": "approve",
			"outputs": [
				{
					"name": "",
					"type": "bool"
				}
			],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`))
}

// updateAllowanceCache updates the cache after a successful approval
func (s *Service) updateAllowanceCache(chainID int, tokenAddr, ownerAddr, spenderAddr common.Address, newAllowance *big.Int) {
	cacheKey := AllowanceCacheKey{
		ChainID:     chainID,
		TokenAddr:   tokenAddr,
		OwnerAddr:   ownerAddr,
		SpenderAddr: spenderAddr,
	}

	now := time.Now()
	s.allowanceMu.Lock()
	s.allowanceCache[cacheKey] = AllowanceCacheEntry{
		Allowance:  newAllowance,
		UpdatedAt:  now,
		Expiration: now.Add(10 * time.Minute), // Cache for 10 minutes
	}
	s.allowanceMu.Unlock()
}

// transactionRecovery runs periodically to recover from stuck transactions
func (s *Service) transactionRecovery(ctx context.Context) {
	log.Println("Transaction recovery job started")

	// Run every 30 minutes
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Transaction recovery job shutting down")
			return
		case <-ticker.C:
			log.Println("Running transaction recovery check")
			s.recoverTransactions(ctx)
		}
	}
}

// recoverTransactions attempts to recover from stuck transactions
func (s *Service) recoverTransactions(ctx context.Context) {
	// Process each chain
	for chainID, chainConfig := range s.config.Chains {
		// First sync nonce state with the blockchain
		err := s.nonceManager.SyncWithBlockchain(
			ctx,
			chainID,
			chainConfig.Client,
			chainConfig.Auth.From,
		)
		if err != nil {
			log.Printf("Failed to sync nonce state for chain %d during recovery: %v", chainID, err)
			continue
		}

		// Find timed out transactions
		timedOutNonces := s.nonceManager.FindTimeoutTransactions(chainID)
		if len(timedOutNonces) == 0 {
			log.Printf("No timed out transactions found for chain %d", chainID)
			continue
		}

		log.Printf("Found %d timed out transactions for chain %d", len(timedOutNonces), chainID)

		// Process each timed out transaction
		for _, nonce := range timedOutNonces {
			// For now, just mark as failed and allow reuse
			log.Printf("Recovering from timed out transaction on chain %d with nonce %d", chainID, nonce)
			s.nonceManager.ReuseNonce(chainID, nonce)

			// Update metrics
			metrics.FailedIntents.WithLabelValues(fmt.Sprintf("%d", chainID)).Inc()
		}

		// After recovery, sync nonce state again
		err = s.nonceManager.SyncWithBlockchain(
			ctx,
			chainID,
			chainConfig.Client,
			chainConfig.Auth.From,
		)
		if err != nil {
			log.Printf("Failed to re-sync nonce state for chain %d after recovery: %v", chainID, err)
		}
	}
}
