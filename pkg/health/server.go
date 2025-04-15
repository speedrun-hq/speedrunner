package health

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/speedrun-hq/fulfiller/pkg/blockchain"
	"github.com/speedrun-hq/fulfiller/pkg/circuitbreaker"
)

// Server represents a health check HTTP server
type Server struct {
	port            string
	chains          map[int]*blockchain.ChainConfig
	circuitBreakers map[int]*circuitbreaker.CircuitBreaker
}

// NewServer creates a new health check server
func NewServer(port string, chains map[int]*blockchain.ChainConfig, circuitBreakers map[int]*circuitbreaker.CircuitBreaker) *Server {
	return &Server{
		port:            port,
		chains:          chains,
		circuitBreakers: circuitBreakers,
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
		for chainID, config := range s.chains {
			if config.Client == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(fmt.Sprintf("Chain %d client not connected", chainID)))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ready"))
	})

	// Chain status endpoint
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := make(map[string]interface{})

		for chainID, config := range s.chains {
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
				blockNumber, err := config.GetLatestBlockNumber(r.Context())
				if err == nil {
					chainStatus["latest_block"] = blockNumber
				}
			}

			status[fmt.Sprintf("chain_%d", chainID)] = chainStatus
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("Error encoding status JSON: %v", err)
		}
	})

	// Circuit breaker admin control endpoint
	http.HandleFunc("/circuit/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
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
			_, _ = w.Write([]byte(fmt.Sprintf("No circuit breaker for chain %d", chainID)))
			return
		}

		cb.Reset()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("Circuit breaker for chain %d reset", chainID)))
	})

	// Expose Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())

	log.Printf("Starting health and metrics server on port %s", s.port)
	if err := http.ListenAndServe(":"+s.port, nil); err != nil {
		log.Printf("Health server error: %v", err)
	}
}
