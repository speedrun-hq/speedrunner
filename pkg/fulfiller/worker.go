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

				// Only retry if circuit not tripped
				if !circuitTripped {
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
						nextAttempt := time.Now().Add(backoff)

						// Create a retry job
						retryJob := models.RetryJob{
							Intent:      intent,
							RetryCount:  retryCount + 1,
							NextAttempt: nextAttempt,
						}

						// Modify intent ID to track retry count
						retryJob.Intent.ID = fmt.Sprintf("%s_retry_%d", parts[0], retryCount+1)

						// Update retry count metric
						metrics.RetryCount.WithLabelValues(strconv.Itoa(intent.DestinationChain)).Inc()

						log.Printf("Scheduling retry for intent %s in %v", intent.ID, backoff)
						s.wg.Add(1)
						s.retryJobs <- retryJob
					} else {
						log.Printf("Max retries reached for intent %s, giving up", intent.ID)
					}
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

// retryHandler manages the retry queue
func (s *Service) retryHandler(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Check retry queue every 10 seconds
	defer ticker.Stop()

	var retryQueue []models.RetryJob

	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.retryJobs:
			// Add to retry queue
			retryQueue = append(retryQueue, job)
		case <-ticker.C:
			now := time.Now()
			var remainingJobs []models.RetryJob

			// Process jobs ready for retry
			for _, job := range retryQueue {
				if job.NextAttempt.Before(now) {
					log.Printf("Retrying intent %s (attempt #%d)", job.Intent.ID, job.RetryCount+1)
					s.wg.Add(1)
					s.pendingJobs <- job.Intent
				} else {
					remainingJobs = append(remainingJobs, job)
				}
			}

			retryQueue = remainingJobs
		}
	}
}
