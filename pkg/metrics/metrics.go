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
		Help: "The number of pending intents waiting to be processed",
	})

	RetryCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_retry_count_total",
		Help: "The total number of retried intent fulfillments by chain",
	}, []string{"chain_id"})

	// New metrics for error tracking
	FulfillmentErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_errors_total",
		Help: "Total number of errors by type",
	}, []string{"chain_id", "error_type"})

	PermanentErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_permanent_errors_total",
		Help: "Total number of permanent errors that won't be retried",
	}, []string{"chain_id", "error_type"})

	// Retry related metrics
	MaxRetriesReached = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_max_retries_reached_total",
		Help: "Number of intents that reached maximum retry attempts",
	}, []string{"chain_id", "error_type"})

	RetryQueueSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fulfiller_retry_queue_size",
		Help: "Current size of the retry queue",
	})

	NextRetryIn = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fulfiller_next_retry_seconds",
		Help: "Seconds until the next scheduled retry",
	})

	RetriesExecuted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_retries_executed_total",
		Help: "Number of retries that were executed",
	}, []string{"chain_id", "error_type"})

	RetriesSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_retries_skipped_total",
		Help: "Number of retries that were skipped",
	}, []string{"chain_id", "reason"})

	DroppedRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_retries_dropped_total",
		Help: "Number of retries that were dropped due to queue capacity",
	}, []string{"chain_id"})

	// FulfilledIntents tracks the number of intents successfully fulfilled by chain
	FulfilledIntents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_fulfilled_intents_total",
		Help: "The total number of successfully fulfilled intents by chain",
	}, []string{"chain_id"})

	// FailedIntents tracks the number of failed intent fulfillments by chain
	FailedIntents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fulfiller_failed_intents_total",
		Help: "The total number of failed intent fulfillments by chain",
	}, []string{"chain_id"})
)
