package fulfiller

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/speedrun-hq/speedrun-fulfiller/pkg/metrics"
	"github.com/speedrun-hq/speedrun-fulfiller/pkg/models"
)

// worker processes intents from the job queue
func (s *Service) worker(ctx context.Context, id int) {
	log.Printf("Starting worker %d", id)
	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down", id)
			return
		case intent, ok := <-s.pendingJobs:
			if !ok {
				// Channel closed
				log.Printf("Worker %d shutting down: channel closed", id)
				return
			}

			// Check if circuit breaker is enabled and open for destination chain
			if cb, ok := s.circuitBreakers[intent.DestinationChain]; ok && cb.IsEnabled() && cb.IsOpen() {
				failureCount, lastFailure, _, _ := cb.GetState()
				log.Printf("Worker %d: Circuit breaker open for chain %d (last failure: %v, failure count: %d), skipping intent %s",
					id, intent.DestinationChain, lastFailure, failureCount, intent.ID)
				s.wg.Done()
				continue
			}

			log.Printf("Worker %d processing intent %s (source: %d, dest: %d, amount: %s)",
				id, intent.ID, intent.SourceChain, intent.DestinationChain, intent.Amount)

			// Record start time for processing duration metric
			startTime := time.Now()

			err := s.fulfillIntent(intent)

			// Record processing time
			processingTime := time.Since(startTime).Seconds()
			metrics.IntentProcessingTime.WithLabelValues(strconv.Itoa(intent.DestinationChain)).Observe(processingTime)

			if err != nil {
				log.Printf("Worker %d error fulfilling intent %s: %v", id, intent.ID, err)

				// Classify error to determine if retry is needed
				shouldRetry, errorType := shouldRetryError(err)

				// Log the error classification
				log.Printf("Error fulfilling intent %s classified as: %s (retry: %v)", intent.ID, errorType, shouldRetry)

				// Track error type in metrics
				metrics.FulfillmentErrors.WithLabelValues(strconv.Itoa(intent.DestinationChain), errorType).Inc()

				// If it's an "already processed" type of error, mark as success and don't retry
				if errorType == "already_processed" {
					log.Printf("Intent %s is already settled or fulfilled, marking as success", intent.ID)
					metrics.IntentsFulfilled.WithLabelValues(strconv.Itoa(intent.DestinationChain), "success").Inc()
					s.wg.Done()
					continue
				}

				// Record failure in circuit breaker
				circuitTripped := false
				if cb, ok := s.circuitBreakers[intent.DestinationChain]; ok {
					circuitTripped = cb.RecordFailure()
					failureCount, _, failureWindow, failThreshold := cb.GetState()
					if circuitTripped {
						log.Printf("Circuit breaker tripped for chain %d - threshold reached: %d failures in %v window",
							intent.DestinationChain, failureCount, failureWindow)
					} else {
						log.Printf("Recorded failure for chain %d - current count: %d/%d in %v window",
							intent.DestinationChain, failureCount, failThreshold, failureWindow)
					}
				}

				// Update metrics for failed intent
				metrics.IntentsFulfilled.WithLabelValues(strconv.Itoa(intent.DestinationChain), "failed").Inc()

				// Only retry if we should retry this error type and circuit is not tripped
				if shouldRetry && !circuitTripped {
					// Check for retry tag in intent ID to determine retry count
					parts := strings.Split(intent.ID, "_retry_")
					retryCount := 0
					if len(parts) > 1 {
						retryCount, _ = strconv.Atoi(parts[1])
					}

					// Only retry up to 3 times
					if retryCount < 3 {
						// Calculate exponential backoff (2^retry * 10 seconds)
						backoff := time.Duration(math.Pow(2, float64(retryCount))) * 10 * time.Second

						// Set a maximum backoff of 2 minutes
						maxBackoff := 2 * time.Minute
						if backoff > maxBackoff {
							backoff = maxBackoff
						}

						nextAttempt := time.Now().Add(backoff)

						// Create a retry job
						retryJob := models.RetryJob{
							Intent:      intent,
							RetryCount:  retryCount + 1,
							NextAttempt: nextAttempt,
						}

						// Store error type in the ID for now (since the field is causing linter issues)
						if errorType != "" {
							// Add error type as a tag to the intent ID
							retryJob.Intent.ID = fmt.Sprintf("%s_retry_%d_error_%s", parts[0], retryCount+1, errorType)
						} else {
							// Standard ID format without error type
							retryJob.Intent.ID = fmt.Sprintf("%s_retry_%d", parts[0], retryCount+1)
						}

						// Update retry count metric
						metrics.RetryCount.WithLabelValues(strconv.Itoa(intent.DestinationChain)).Inc()

						log.Printf("Scheduling retry for intent %s in %v (error: %s)", intent.ID, backoff, errorType)
						s.wg.Add(1)
						s.retryJobs <- retryJob
					} else {
						log.Printf("Max retries reached for intent %s, giving up (error: %s)", intent.ID, errorType)
						metrics.MaxRetriesReached.WithLabelValues(strconv.Itoa(intent.DestinationChain), errorType).Inc()
					}
				} else if !shouldRetry {
					log.Printf("Not retrying intent %s due to permanent error type: %s", intent.ID, errorType)
					metrics.PermanentErrors.WithLabelValues(strconv.Itoa(intent.DestinationChain), errorType).Inc()
				} else {
					log.Printf("Skipping retry for intent %s due to tripped circuit breaker", intent.ID)
				}
			} else {
				log.Printf("Worker %d successfully fulfilled intent %s", id, intent.ID)
				// Update metrics for successful intent
				metrics.IntentsFulfilled.WithLabelValues(strconv.Itoa(intent.DestinationChain), "success").Inc()
			}
			s.wg.Done()
		}
	}
}

// shouldRetryError classifies errors to determine if a retry should be attempted
// Returns (shouldRetry, errorType)
func shouldRetryError(err error) (bool, string) {
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
		return false, "insufficient_funds"
	}

	// Contract errors - likely permanent failures
	if strings.Contains(errStr, "execution reverted") {
		return false, "contract_error"
	}

	// Any other error - retry by default
	return true, "unknown_error"
}
