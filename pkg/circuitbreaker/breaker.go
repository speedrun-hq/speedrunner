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
		if time.Since(cb.tripTime) > cb.resetTimeout {
			log.Printf("Circuit breaker: Attempting to reset after timeout")
			cb.tripped = false
			cb.failureCount = 0
		} else {
			return true // Still tripped
		}
	}

	// Reset failure count if outside window
	if time.Since(cb.lastFailure) > cb.failureWindow {
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
	if cb.tripped && time.Since(cb.tripTime) > cb.resetTimeout {
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

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() (failureCount int, lastFailure time.Time, failureWindow time.Duration, failThreshold int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failureCount, cb.lastFailure, cb.failureWindow, cb.failThreshold
}

// GetTripTime returns the time when the circuit was tripped
func (cb *CircuitBreaker) GetTripTime() time.Time {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.tripTime
}

// IsEnabled returns true if the circuit breaker is enabled
func (cb *CircuitBreaker) IsEnabled() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.enabled
}
