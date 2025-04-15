package circuitbreaker

import (
	"log"
	"sync"
	"time"
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	enabled       bool
	failureCount  int
	failureWindow time.Duration
	failThreshold int
	resetTimeout  time.Duration
	lastFailure   time.Time
	tripped       bool
	tripTime      time.Time
	mu            sync.Mutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(enabled bool, threshold int, window time.Duration, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		enabled:       enabled,
		failThreshold: threshold,
		failureWindow: window,
		resetTimeout:  resetTimeout,
	}
}

// RecordFailure records a failure and trips the circuit if threshold is exceeded
func (cb *CircuitBreaker) RecordFailure() bool {
	if !cb.enabled {
		return false
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// If the circuit is already tripped, check if it's time to try again
	if cb.tripped {
		if now.Sub(cb.tripTime) > cb.resetTimeout {
			log.Printf("Circuit breaker: Attempting to reset after timeout")
			cb.tripped = false
			cb.failureCount = 0
		} else {
			return true // Still tripped
		}
	}

	// Reset failure count if outside window
	if now.Sub(cb.lastFailure) > cb.failureWindow {
		cb.failureCount = 0
	}

	// Record this failure
	cb.failureCount++
	cb.lastFailure = now

	// Check if we need to trip the circuit
	if cb.failureCount >= cb.failThreshold {
		cb.tripped = true
		cb.tripTime = now
		log.Printf("Circuit breaker tripped: %d failures in window", cb.failureCount)
		return true
	}

	return false
}

// IsOpen returns true if the circuit is open (tripped)
func (cb *CircuitBreaker) IsOpen() bool {
	if !cb.enabled {
		return false
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// If tripped but reset timeout has passed, try again
	if cb.tripped && time.Now().Sub(cb.tripTime) > cb.resetTimeout {
		cb.tripped = false
		cb.failureCount = 0
		return false
	}

	return cb.tripped
}

// Reset manually resets the circuit breaker
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.tripped = false
	cb.failureCount = 0
}
