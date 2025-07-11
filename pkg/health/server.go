package health

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/speedrun-hq/speedrunner/pkg/chainclient"
	"github.com/speedrun-hq/speedrunner/pkg/chains"
	"github.com/speedrun-hq/speedrunner/pkg/circuitbreaker"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
)

// Server represents a health check HTTP server
type Server struct {
	port            string
	chains          map[int]*chainclient.Client
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
	metricsAPIKey   string
	logger          logger.Logger
}

// NewServer creates a new health check server
func NewServer(
	port string,
	chains map[int]*chainclient.Client,
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker,
	logger logger.Logger,
) *Server {
	return &Server{
		port:            port,
		chains:          chains,
		circuitBreakers: circuitBreakers,
		metricsAPIKey:   os.Getenv("METRICS_API_KEY"),
		logger:          logger,
	}
}

// Start starts the health check server
func (s *Server) Start() {
	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Readiness check
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check if all chain clients are connected
		for chainID, chainConfig := range s.chains {
			if chainConfig.Client == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "Chain %d client not connected", chainID)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ready"))
	})

	// Chain status endpoint
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := make(map[string]interface{})

		for chainID, chainConfig := range s.chains {
			status[fmt.Sprintf("chain_%d", chainID)] = s.getChainStatus(r.Context(), chainID, chainConfig)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			s.logger.Error("Error encoding status JSON: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Failed to encode status"))
		}
	})

	// Circuit breaker admin control endpoint
	http.HandleFunc("/circuit/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("Method not allowed"))
			return
		}

		chainIDStr := r.URL.Query().Get("chain")
		if chainIDStr == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Missing chain parameter"))
			return
		}

		chainID, err := strconv.Atoi(chainIDStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid chain ID"))
			return
		}

		cb, ok := s.circuitBreakers[chainID]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, "No circuit breaker for chain %d", chainID)
			return
		}

		cb.Reset()
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "Circuit breaker for chain %d reset", chainID)
	})

	// Expose Prometheus metrics with API key authentication
	http.Handle("/metrics", s.metricsAuthMiddleware(promhttp.Handler()))

	s.logger.Notice("Starting health and metrics server on port %s", s.port)
	if err := http.ListenAndServe(":"+s.port, nil); err != nil {
		s.logger.Error("Health server error: %v", err)
	}
}

// metricsAuthMiddleware is a middleware that checks for a valid API key
func (s *Server) metricsAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if no API key is configured
		if s.metricsAPIKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Get API key from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Check if the header has the correct format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		// Validate API key
		if parts[1] != s.metricsAPIKey {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getTokenBalances retrieves balances for configured tokens on a chain
func (s *Server) getTokenBalances(ctx context.Context, chainID int, chainConfig *chainclient.Client) map[string]interface{} {
	tokenBalances := make(map[string]interface{})

	chainName := chains.GetChainName(chainID)
	if chainName == "" {
		s.logger.Info("Warning: Unknown chain ID %d", chainID)
		return tokenBalances
	}

	// Get USDC balance
	if usdcAddr := chains.GetTokenAddress(chainID, chains.TokenTypeUSDC); usdcAddr != "" {
		if balance, err := s.getTokenBalance(ctx, chainConfig.Client, common.HexToAddress(usdcAddr), chainConfig.Auth.From); err == nil {
			tokenBalances["USDC"] = balance.String()
		} else {
			s.logger.Info("Warning: Failed to get USDC balance for chain %s: %v", chainName, err)
		}
	} else {
		s.logger.Info("Warning: No USDC address configured for chain %s", chainName)
	}

	// Get USDT balance
	if usdtAddr := chains.GetTokenAddress(chainID, chains.TokenTypeUSDT); usdtAddr != "" {
		if balance, err := s.getTokenBalance(ctx, chainConfig.Client, common.HexToAddress(usdtAddr), chainConfig.Auth.From); err == nil {
			tokenBalances["USDT"] = balance.String()
		} else {
			s.logger.Info("Warning: Failed to get USDT balance for chain %s: %v", chainName, err)
		}
	} else {
		s.logger.Info("Warning: No USDT address configured for chain %s", chainName)
	}

	return tokenBalances
}

// getChainStatus returns the status information for a specific chain
func (s *Server) getChainStatus(ctx context.Context, chainID int, config *chainclient.Client) map[string]interface{} {
	circuitStatus := "closed"
	if cb, ok := s.circuitBreakers[chainID]; ok && cb.IsOpen() {
		circuitStatus = "open"
	}

	chainStatus := map[string]interface{}{
		"rpc_url":        config.RPCURL,
		"intent_address": config.IntentAddress,
		"connected":      config.Client != nil,
		"circuit":        circuitStatus,
	}

	// Get latest block number if connected
	if config.Client != nil {
		blockNumber, err := config.GetLatestBlockNumber(ctx)
		if err == nil {
			chainStatus["latest_block"] = blockNumber
		} else {
			s.logger.InfoWithChain(chainID, "Warning: Failed to get latest block for chain %d: %v", err)
		}

		// Get token balances
		if tokenBalances := s.getTokenBalances(ctx, chainID, config); len(tokenBalances) > 0 {
			chainStatus["token_balances"] = tokenBalances
		}
	}

	return chainStatus
}

// getTokenBalance retrieves the token balance for a given address
func (s *Server) getTokenBalance(ctx context.Context, client *ethclient.Client, tokenAddress, ownerAddress common.Address) (*big.Int, error) {
	token, err := contracts.NewERC20(tokenAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create token contract: %v", err)
	}

	balance, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, ownerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get token balance: %v", err)
	}

	// Get token symbol and decimals for metrics
	symbol := "UNKNOWN"
	decimals := uint8(18) // Default to 18 decimals if we can't get the actual value

	// Try to get symbol, but don't fail if we can't
	if symbolResult, err := token.Symbol(&bind.CallOpts{Context: ctx}); err == nil {
		symbol = symbolResult
	} else {
		s.logger.Info("Warning: Failed to get token symbol for %s: %v", tokenAddress.Hex(), err)
	}

	// Try to get decimals, but don't fail if we can't
	if decimalsResult, err := token.Decimals(&bind.CallOpts{Context: ctx}); err == nil {
		decimals = decimalsResult
	} else {
		// TODO: error might need to be handled here
		s.logger.Info("Warning: Failed to get token decimals for %s: %v", tokenAddress.Hex(), err)
	}

	// Convert balance to float64 for Prometheus metric
	balanceFloat := new(big.Float).SetInt(balance)
	decimalsMultiplier := new(big.Float).SetInt64(10)
	decimalsMultiplier = new(big.Float).Mul(decimalsMultiplier, new(big.Float).SetInt64(int64(decimals)))
	balanceFloat.Quo(balanceFloat, decimalsMultiplier)
	balanceFloat64, _ := balanceFloat.Float64()

	// Get chain ID
	chainID, err := client.ChainID(ctx)
	if err != nil {
		s.logger.Info("Warning: Failed to get chain ID: %v", err)
		return balance, nil // Return balance even if we can't get chain ID
	}

	// Update Prometheus metric
	metrics.TokenBalance.WithLabelValues(
		chains.GetChainName(int(chainID.Int64())),
		symbol,
	).Set(balanceFloat64)

	return balance, nil
}
