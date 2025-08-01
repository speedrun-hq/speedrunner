package chainclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/logger"
)

// FeeUpdateRoutine manages the periodic updates of gas price, token price, and withdraw fee
type FeeUpdateRoutine struct {
	ctx      context.Context
	client   *Client
	interval time.Duration
	stopChan chan struct{}
	mu       sync.RWMutex
	running  bool
	logger   logger.Logger
}

// NewFeeUpdateRoutine creates a new fee update routine
func NewFeeUpdateRoutine(client *Client, interval time.Duration) *FeeUpdateRoutine {
	return &FeeUpdateRoutine{
		ctx:      client.Ctx,
		client:   client,
		interval: interval,
		stopChan: nil,
		running:  false,
		logger:   client.logger,
	}
}

// Start begins the periodic fee updates
func (r *FeeUpdateRoutine) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return // Already running
	}

	r.stopChan = make(chan struct{})
	r.running = true

	go r.run()
}

// Stop halts the periodic fee updates
func (r *FeeUpdateRoutine) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	close(r.stopChan)
	r.stopChan = nil
	r.running = false
}

// IsRunning returns whether the routine is currently running
func (r *FeeUpdateRoutine) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// run is the main goroutine that performs periodic updates
func (r *FeeUpdateRoutine) run() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Perform initial update
	if err := r.updatePrices(); err != nil {
		r.logger.ErrorWithChain(r.client.ChainID, "Failed to perform initial fee update: %v", err)
		return
	}

	for {
		select {
		case <-ticker.C:
			if err := r.updatePrices(); err != nil {
				r.logger.ErrorWithChain(r.client.ChainID, "Failed to perform initial fee update: %v", err)
				return
			}
		case <-r.stopChan:
			return
		}
	}
}

// updatePrices performs a single update of gas price, token price, and withdraw fee
func (r *FeeUpdateRoutine) updatePrices() error {
	// Update gas price
	gasPrice, err := r.client.UpdateGasPrice(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to update gas price: %v", err)
	}

	// Update token price
	tokenPrice, err := getTokenPriceUSD(r.ctx, r.client.ChainID)
	if err != nil {
		return fmt.Errorf("failed to fetch token price for chain %d: %v", r.client.ChainID, err)
	}

	// Compute withdraw fee
	withdrawFee := computeWithdrawFee(gasPrice, tokenPrice)

	// Store the values in the client
	r.client.mu.Lock()
	r.client.CurrentGasPrice = gasPrice
	r.client.TokenPriceUSD = tokenPrice
	r.client.WithdrawFeeUSD = withdrawFee
	r.client.mu.Unlock()

	// Log the updated values
	r.logger.InfoWithChain(r.client.ChainID,
		"Updated gas price: %s, Token price: $%.2f, Withdraw fee: $%.2f",
		gasPrice.String(),
		tokenPrice,
		withdrawFee,
	)

	// TODO: Implement metrics updates

	return nil
}

// getTokenPriceUSD fetches the current USD price for the gas token of a specific chain
func getTokenPriceUSD(ctx context.Context, chainID int) (float64, error) {
	// Map chain IDs to CoinGecko token IDs
	tokenIDs := map[int]string{
		1:     "ethereum",      // Ethereum
		137:   "matic-network", // Polygon
		42161: "ethereum",      // Arbitrum (uses ETH)
		8453:  "ethereum",      // Base (uses ETH)
		56:    "binancecoin",   // BSC
		43114: "avalanche-2",   // Avalanche
		7000:  "zetachain",     // ZetaChain
	}

	tokenID, exists := tokenIDs[chainID]
	if !exists {
		return 0, fmt.Errorf("unsupported chain ID for price fetching: %d", chainID)
	}

	// Check cache first
	cache := getOrCreateCache()
	if cachedPrice, found := cache.Get(tokenID); found {
		return cachedPrice, nil
	}

	// Fetch price from CoinGecko API
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", tokenID)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(timeoutCtx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch token price: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %v", err)
	}

	var result map[string]map[string]float64
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	tokenData, exists := result[tokenID]
	if !exists {
		return 0, fmt.Errorf("token data not found in response")
	}

	price, exists := tokenData["usd"]
	if !exists {
		return 0, fmt.Errorf("USD price not found in response")
	}

	// Cache the price for future use
	cache.Set(tokenID, price)

	return price, nil
}

// computeWithdrawFee calculates the withdraw fee in USD using the formula: gasPrice * 100000
func computeWithdrawFee(gasPrice *big.Int, tokenPriceUSD float64) float64 {
	// Handle nil gas price
	if gasPrice == nil {
		return 0.0
	}

	// Convert gas price to float64 (assuming gas price is in wei)
	gasPriceFloat := new(big.Float).SetInt(gasPrice)

	// Calculate: gasPrice * 100000
	multiplier := big.NewFloat(100000)
	result := new(big.Float).Mul(gasPriceFloat, multiplier)

	// Convert to float64
	withdrawFeeWei, _ := result.Float64()

	// Convert from wei to USD: (wei / 10^18) * tokenPriceUSD
	weiToEth := 1e18
	withdrawFeeUSD := (withdrawFeeWei / weiToEth) * tokenPriceUSD

	return withdrawFeeUSD
}
