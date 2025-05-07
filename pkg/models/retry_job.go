package models

import (
	"time"
)

// RetryJob represents a job that needs to be retried
type RetryJob struct {
	Intent      Intent
	RetryCount  int
	NextAttempt time.Time
	ErrorType   string // Type of error that caused the retry
}
