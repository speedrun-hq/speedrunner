package fulfiller

import (
	"context"
	"math/big"
	"strconv"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/config"
	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
)

// MetricsManager handles metrics collection and monitoring
type MetricsManager struct {
	config *config.Config
	logger logger.Logger
}

// NewMetricsManager creates a new metrics manager
func NewMetricsManager(config *config.Config, logger logger.Logger) *MetricsManager {
	return &MetricsManager{
		config: config,
		logger: logger,
	}
}

// UpdateMetrics updates all metrics
func (mm *MetricsManager) UpdateMetrics() {
	mm.logger.Debug("Updating metrics...")

	// Update gas prices for each chain
	for chainID, chainConfig := range mm.config.Chains {
		gasPrice, err := chainConfig.UpdateGasPrice(context.Background())
		if err != nil {
			mm.logger.DebugWithChain(chainID, "Failed to update gas price: %v", err)
			continue
		}

		// Convert to gwei for readability
		gasPriceGwei := new(big.Float).Quo(
			new(big.Float).SetInt(gasPrice),
			big.NewFloat(1e9), // 1 gwei = 10^9 wei
		)
		gweiFlt, _ := gasPriceGwei.Float64()
		metrics.GasPrice.WithLabelValues(strconv.Itoa(chainID)).Set(gweiFlt)

		mm.logger.DebugWithChain(chainID, "Updated gas price: %.2f gwei", gweiFlt)
	}

	// Update pending intents count
	// This would typically be fetched from the API
	// For now, we'll set it to 0 as a placeholder
	metrics.PendingIntents.Set(0)

	// Update other metrics as needed
	mm.logger.Debug("Metrics update completed")
}

// StartMetricsUpdater starts the metrics updater goroutine
func (mm *MetricsManager) StartMetricsUpdater(ctx context.Context) {
	mm.logger.Info("Starting metrics updater")
	ticker := time.NewTicker(30 * time.Second) // Update metrics every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			mm.logger.Info("Metrics updater shutting down")
			return
		case <-ticker.C:
			mm.UpdateMetrics()
		}
	}
}

// IsGasPriceAcceptable checks if the gas price is acceptable for a given chain
func (mm *MetricsManager) IsGasPriceAcceptable(chainID int) bool {
	chainConfig, exists := mm.config.Chains[chainID]
	if !exists {
		mm.logger.DebugWithChain(chainID, "Chain configuration not found")
		return false
	}

	// Get current gas price
	gasPrice, err := chainConfig.UpdateGasPrice(context.Background())
	if err != nil {
		mm.logger.DebugWithChain(chainID, "Failed to get gas price: %v", err)
		return false
	}

	// Check if gas price is within acceptable range
	// This is a simplified check - in practice, you might want more sophisticated logic
	maxGasPrice := big.NewInt(100 * 1e9) // 100 gwei
	if gasPrice.Cmp(maxGasPrice) > 0 {
		mm.logger.DebugWithChain(chainID, "Gas price too high: %s gwei",
			new(big.Float).Quo(new(big.Float).SetInt(gasPrice), big.NewFloat(1e9)))
		return false
	}

	return true
}
