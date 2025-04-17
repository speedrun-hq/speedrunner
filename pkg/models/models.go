package models

import (
	"time"
)

// Intent represents an intent to be fulfilled
type Intent struct {
	ID               string `json:"id"`
	SourceChain      int    `json:"source_chain"`
	DestinationChain int    `json:"destination_chain"`
	Amount           string `json:"amount"`
	IntentFee        string `json:"intent_fee"`
	TokenSymbol      string `json:"token_symbol"`
	TokenAddress     string `json:"token_address"`
	Recipient        string `json:"recipient"`
	Status           string `json:"status"`
}

// RetryJob represents a scheduled retry for an intent
type RetryJob struct {
	Intent      Intent
	RetryCount  int
	NextAttempt time.Time
	ErrorType   string // Type of error that caused the retry
}
