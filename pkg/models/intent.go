package models

import (
	"time"
)

// Intent represents an intent from the API
type Intent struct {
	ID               string    `json:"id"`
	SourceChain      int       `json:"source_chain"`
	DestinationChain int       `json:"destination_chain"`
	Token            string    `json:"token"`
	Amount           string    `json:"amount"`
	Recipient        string    `json:"recipient"`
	IntentFee        string    `json:"intent_fee"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RetryJob represents a job that needs to be retried
type RetryJob struct {
	Intent      Intent
	RetryCount  int
	NextAttempt time.Time
}
