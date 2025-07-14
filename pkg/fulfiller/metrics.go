package fulfiller

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/speedrun-hq/speedrunner/pkg/chains"
	"github.com/speedrun-hq/speedrunner/pkg/contracts"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
)

// startMetricsUpdater starts a goroutine to update metrics periodically
func (s *Service) startMetricsUpdater(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateMetrics(ctx)
		}
	}
}

// updateMetrics updates Prometheus metrics
func (s *Service) updateMetrics(ctx context.Context) {
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
			token, err := contracts.NewERC20(tokenAddress, s.chainClients[chainID].Client)
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
	for chainID, chainConfig := range s.chainClients {
		chainName := chains.GetChainName(chainID)
		if chainName == "" {
			chainName = "Unknown"
		}

		gasPrice, err := chainConfig.Client.SuggestGasPrice(ctx)
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
