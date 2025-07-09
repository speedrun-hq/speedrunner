package fulfiller

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/logger"
	"github.com/speedrun-hq/speedrunner/pkg/metrics"
	"github.com/speedrun-hq/speedrunner/pkg/models"
)

// RetryManager handles retry logic and job scheduling
type RetryManager struct {
	retryJobs chan models.RetryJob
	logger    logger.Logger
}

// NewRetryManager creates a new retry manager
func NewRetryManager(logger logger.Logger) *RetryManager {
	return &RetryManager{
		retryJobs: make(chan models.RetryJob, 100), // Buffer for retry jobs
		logger:    logger,
	}
}

// GetRetryJobsChannel returns the retry jobs channel
func (rm *RetryManager) GetRetryJobsChannel() chan models.RetryJob {
	return rm.retryJobs
}

// StartRetryHandler starts the retry handler goroutine
func (rm *RetryManager) StartRetryHandler(ctx context.Context) {
	rm.logger.Info("Starting retry handler")
	go rm.retryHandler(ctx)
}

// retryHandler processes retry jobs
func (rm *RetryManager) retryHandler(ctx context.Context) {
	rm.logger.Info("Retry handler started")
	for {
		select {
		case <-ctx.Done():
			rm.logger.Info("Retry handler shutting down")
			return
		case <-time.After(1 * time.Second):
			rm.processRetryJobs()
		}
	}
}

// processRetryJobs processes retry jobs that are ready to be retried
func (rm *RetryManager) processRetryJobs() {
	// This would typically iterate through stored retry jobs and check if they're ready
	// For now, this is a placeholder implementation
	rm.logger.Debug("Processing retry jobs...")
}

// ShouldRetryError classifies errors to determine if a retry should be attempted
// Returns (shouldRetry, errorType)
func (rm *RetryManager) ShouldRetryError(err error) (bool, string) {
	errStr := err.Error()

	// Check for "already processed" errors - no retry needed
	if strings.Contains(errStr, "Intent already settled") ||
		strings.Contains(errStr, "Intent already fulfilled") ||
		strings.Contains(errStr, "already fulfilled with these parameters") {
		return false, "already_processed"
	}

	// Network/RPC errors - retry is appropriate
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "no response") ||
		strings.Contains(errStr, "EOF") {
		return true, "network_error"
	}

	// RPC node state errors - retry with longer backoff
	if strings.Contains(errStr, "missing trie node") ||
		strings.Contains(errStr, "layer stale") ||
		strings.Contains(errStr, "getDeleteStateObject") ||
		strings.Contains(errStr, "state inconsistency") ||
		strings.Contains(errStr, "receipt not found") ||
		strings.Contains(errStr, "block not found") {
		return true, "node_state_error"
	}

	// Gas-related errors - retry may help if gas prices change
	if strings.Contains(errStr, "gas required exceeds allowance") ||
		strings.Contains(errStr, "insufficient funds for gas") ||
		strings.Contains(errStr, "gas price too low") {
		return true, "gas_error"
	}

	// Nonce-related errors - retry may help after nonce is corrected
	if strings.Contains(errStr, "nonce too low") ||
		strings.Contains(errStr, "nonce too high") ||
		strings.Contains(errStr, "replacement transaction underpriced") {
		return true, "nonce_error"
	}

	// Balance-related errors - permanent failures
	if strings.Contains(errStr, "insufficient balance") ||
		strings.Contains(errStr, "insufficient funds") {
		return false, "insufficient_balance"
	}

	// Contract-related errors - permanent failures
	if strings.Contains(errStr, "execution reverted") ||
		strings.Contains(errStr, "invalid opcode") ||
		strings.Contains(errStr, "out of gas") {
		return false, "contract_error"
	}

	// Unknown errors - retry with caution
	return true, "unknown_error"
}

// CalculateBackoff calculates the backoff duration for retry attempts
func (rm *RetryManager) CalculateBackoff(retryCount int) time.Duration {
	// Calculate exponential backoff (2^retry * 10 seconds)
	backoff := time.Duration(math.Pow(2, float64(retryCount))) * 10 * time.Second

	// Set a maximum backoff of 2 minutes
	maxBackoff := 2 * time.Minute
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return backoff
}

// ScheduleRetry schedules a retry for an intent
func (rm *RetryManager) ScheduleRetry(intent models.Intent, retryCount int, errorType string) {
	// Check for retry tag in intent ID to determine retry count
	parts := strings.Split(intent.ID, "_retry_")
	currentRetryCount := 0
	if len(parts) > 1 {
		currentRetryCount, _ = strconv.Atoi(parts[1])
	}

	// Only retry up to 3 times
	if currentRetryCount >= 3 {
		rm.logger.Info("Max retries reached for intent %s, giving up (error: %s)", intent.ID, errorType)
		metrics.MaxRetriesReached.WithLabelValues(strconv.Itoa(intent.DestinationChain), errorType).Inc()
		return
	}

	// Calculate backoff
	backoff := rm.CalculateBackoff(currentRetryCount)
	nextAttempt := time.Now().Add(backoff)

	// Create a retry job
	retryJob := models.RetryJob{
		Intent:      intent,
		RetryCount:  currentRetryCount + 1,
		NextAttempt: nextAttempt,
	}

	// Add error type as a tag to the intent ID
	if errorType != "" {
		retryJob.Intent.ID = strings.Join(parts[:len(parts)-1], "_retry_") + "_retry_" + strconv.Itoa(currentRetryCount+1) + "_error_" + errorType
	} else {
		// Standard ID format without error type
		retryJob.Intent.ID = strings.Join(parts[:len(parts)-1], "_retry_") + "_retry_" + strconv.Itoa(currentRetryCount+1)
	}

	// Update retry count metric
	metrics.RetryCount.WithLabelValues(strconv.Itoa(intent.DestinationChain)).Inc()

	rm.logger.Info("Scheduling retry for intent %s in %v (error: %s)", intent.ID, backoff, errorType)
	rm.retryJobs <- retryJob
}
