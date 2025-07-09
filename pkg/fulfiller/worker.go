package fulfiller

import (
	"context"
	"strconv"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/metrics"
)

// worker processes intents from the job queue
func (s *Service) worker(ctx context.Context, id int) {
	s.logger.Info("Starting worker %d", id)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Worker %d shutting down", id)
			return
		case intent, ok := <-s.pendingJobs:
			if !ok {
				// Channel closed
				s.logger.Info("Worker %d shutting down: channel closed", id)
				return
			}

			// Check if circuit breaker is enabled and open for destination chain
			if cb, ok := s.circuitBreakers[intent.DestinationChain]; ok && cb.IsEnabled() && cb.IsOpen() {
				failureCount, lastFailure, _, _ := cb.GetState()
				s.logger.Info("Worker %d: Circuit breaker open for chain %d (last failure: %v, failure count: %d), skipping intent %s",
					id, intent.DestinationChain, lastFailure, failureCount, intent.ID)
				s.wg.Done()
				continue
			}

			s.logger.Info("Worker %d processing intent %s (source: %d, dest: %d, amount: %s)",
				id, intent.ID, intent.SourceChain, intent.DestinationChain, intent.Amount)

			// Record start time for processing duration metric
			startTime := time.Now()

			err := s.fulfillIntent(intent)

			// Record processing time
			processingTime := time.Since(startTime).Seconds()
			metrics.IntentProcessingTime.WithLabelValues(strconv.Itoa(intent.DestinationChain)).Observe(processingTime)

			if err != nil {
				s.logger.Info("Worker %d error fulfilling intent %s: %v", id, intent.ID, err)

				// Classify error to determine if retry is needed
				shouldRetry, errorType := s.retryManager.ShouldRetryError(err)

				// Log the error classification
				s.logger.Info("Error fulfilling intent %s classified as: %s (retry: %v)", intent.ID, errorType, shouldRetry)

				// Track error type in metrics
				metrics.FulfillmentErrors.WithLabelValues(strconv.Itoa(intent.DestinationChain), errorType).Inc()

				// If it's an "already processed" type of error, mark as success and don't retry
				if errorType == "already_processed" {
					s.logger.Info("Intent %s is already settled or fulfilled, marking as success", intent.ID)
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
						s.logger.Info("Circuit breaker tripped for chain %d - threshold reached: %d failures in %v window",
							intent.DestinationChain, failureCount, failureWindow)
					} else {
						s.logger.Info("Recorded failure for chain %d - current count: %d/%d in %v window",
							intent.DestinationChain, failureCount, failThreshold, failureWindow)
					}
				}

				// Update metrics for failed intent
				metrics.IntentsFulfilled.WithLabelValues(strconv.Itoa(intent.DestinationChain), "failed").Inc()

				// Only retry if we should retry this error type and circuit is not tripped
				if shouldRetry && !circuitTripped {
					// Use the retry manager to schedule the retry
					s.retryManager.ScheduleRetry(intent, 0, errorType)
				} else if !shouldRetry {
					s.logger.Info("Not retrying intent %s due to permanent error type: %s", intent.ID, errorType)
					metrics.PermanentErrors.WithLabelValues(strconv.Itoa(intent.DestinationChain), errorType).Inc()
				} else {
					s.logger.Info("Skipping retry for intent %s due to tripped circuit breaker", intent.ID)
				}
			} else {
				s.logger.Info("Worker %d successfully fulfilled intent %s", id, intent.ID)
				// Update metrics for successful intent
				metrics.IntentsFulfilled.WithLabelValues(strconv.Itoa(intent.DestinationChain), "success").Inc()
			}
			s.wg.Done()
		}
	}
}
