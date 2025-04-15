package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for monitoring
var (
	IntentsFulfilled = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_intents_fulfilled_total",
		Help: "The total number of fulfilled intents",
	}, []string{"chain_id", "status"})

	IntentProcessingTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "fulfiller_intent_processing_seconds",
		Help:    "Time taken to process intents",
		Buckets: prometheus.ExponentialBuckets(1, 2, 10), // Start at 1s with 10 buckets doubling in size
	}, []string{"chain_id"})

	GasUsed = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "fulfiller_gas_used",
		Help:    "Gas used for fulfilling intents",
		Buckets: prometheus.ExponentialBuckets(21000, 2, 10), // Start at 21000 with 10 buckets doubling in size
	}, []string{"chain_id"})

	GasPrice = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fulfiller_gas_price_gwei",
		Help: "Current gas price in gwei",
	}, []string{"chain_id"})

	PendingIntents = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fulfiller_pending_intents",
		Help: "Number of intents pending fulfillment",
	})

	RetryCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_retry_count_total",
		Help: "Total number of retry attempts",
	}, []string{"chain_id"})
)
