package blockchain

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus int

const (
	// TxPending indicates transaction is pending
	TxPending TransactionStatus = iota
	// TxConfirmed indicates transaction is confirmed
	TxConfirmed
	// TxFailed indicates transaction has failed
	TxFailed
	// TxTimedOut indicates transaction has timed out
	TxTimedOut
)

// TransactionRecord tracks details about a transaction
type TransactionRecord struct {
	Hash       common.Hash
	Nonce      uint64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Status     TransactionStatus
	RetryCount int
}

// NonceManager handles nonce allocation and tracking
type NonceManager struct {
	// Per-chain data structures
	chains map[int]*chainNonceData
	// Global lock for accessing chains map
	mu sync.RWMutex
	// Transaction timeout duration
	txTimeout time.Duration
}

// chainNonceData holds nonce data for a specific chain
type chainNonceData struct {
	// Current nonce counter
	currentNonce uint64
	// Map of pending transactions by nonce
	pendingTxs map[uint64]*TransactionRecord
	// Last time nonce was synchronized with the blockchain
	lastSync time.Time
	// Chain-specific mutex for nonce operations
	mu sync.Mutex
}

// NewNonceManager creates a new nonce manager
func NewNonceManager() *NonceManager {
	return &NonceManager{
		chains:    make(map[int]*chainNonceData),
		txTimeout: 5 * time.Minute, // Default timeout of 5 minutes
	}
}

// SetTransactionTimeout sets the timeout for transactions
func (nm *NonceManager) SetTransactionTimeout(timeout time.Duration) {
	nm.txTimeout = timeout
}

// initializeChain ensures chain data is initialized
func (nm *NonceManager) initializeChain(chainID int) {
	nm.mu.RLock()
	_, exists := nm.chains[chainID]
	nm.mu.RUnlock()

	if !exists {
		nm.mu.Lock()
		nm.chains[chainID] = &chainNonceData{
			currentNonce: 0,
			pendingTxs:   make(map[uint64]*TransactionRecord),
			lastSync:     time.Time{},
		}
		nm.mu.Unlock()
	}
}

// GetNonce reserves and returns the next available nonce
func (nm *NonceManager) GetNonce(ctx context.Context, chainID int, client *ethclient.Client, address common.Address) (uint64, error) {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// If nonce hasn't been initialized or it's been more than 5 minutes since last sync
	if chainData.lastSync.IsZero() || time.Since(chainData.lastSync) > 5*time.Minute {
		// Fetch the current nonce from the blockchain
		nonce, err := client.PendingNonceAt(ctx, address)
		if err != nil {
			return 0, fmt.Errorf("failed to get pending nonce: %v", err)
		}

		// If our tracked nonce is behind, update it
		if nonce > chainData.currentNonce {
			log.Printf("Updating nonce for chain %d: %d -> %d", chainID, chainData.currentNonce, nonce)
			chainData.currentNonce = nonce
		}
		chainData.lastSync = time.Now()
	}

	// Allocate the nonce
	nonce := chainData.currentNonce
	chainData.currentNonce++

	// Return the allocated nonce
	return nonce, nil
}

// TrackTransaction records a new transaction
func (nm *NonceManager) TrackTransaction(chainID int, txHash common.Hash, nonce uint64) {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// Record the transaction
	now := time.Now()
	chainData.pendingTxs[nonce] = &TransactionRecord{
		Hash:      txHash,
		Nonce:     nonce,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    TxPending,
	}

	log.Printf("Tracking transaction for chain %d with nonce %d: %s", chainID, nonce, txHash.Hex())
}

// MarkTransactionConfirmed marks a transaction as confirmed
func (nm *NonceManager) MarkTransactionConfirmed(chainID int, nonce uint64) bool {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// Check if transaction exists
	tx, exists := chainData.pendingTxs[nonce]
	if !exists {
		log.Printf("Warning: No pending transaction found for chain %d, nonce %d", chainID, nonce)
		return false
	}

	// Update the transaction
	tx.Status = TxConfirmed
	tx.UpdatedAt = time.Now()
	log.Printf("Transaction confirmed for chain %d, nonce %d: %s", chainID, nonce, tx.Hash.Hex())

	// Remove the transaction from pending
	delete(chainData.pendingTxs, nonce)
	return true
}

// MarkTransactionFailed marks a transaction as failed
func (nm *NonceManager) MarkTransactionFailed(chainID int, nonce uint64) uint64 {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// Check if transaction exists
	tx, exists := chainData.pendingTxs[nonce]
	if !exists {
		log.Printf("Warning: No pending transaction found for chain %d, nonce %d", chainID, nonce)
		return 0
	}

	// Update the transaction
	tx.Status = TxFailed
	tx.UpdatedAt = time.Now()
	log.Printf("Transaction failed for chain %d, nonce %d: %s", chainID, nonce, tx.Hash.Hex())

	// If this was the lowest pending nonce, we need to reuse it
	lowestPending := nm.getLowestPendingNonce(chainData)
	if nonce == lowestPending {
		// We can reuse this nonce since the transaction failed
		// and we have no lower nonces pending
		chainData.currentNonce = nonce
		log.Printf("Reusing nonce %d for chain %d after transaction failure", nonce, chainID)
		delete(chainData.pendingTxs, nonce)
		return nonce
	}

	// Otherwise just mark as failed but don't change nonce allocation
	delete(chainData.pendingTxs, nonce)
	return 0
}

// FindTimeoutTransactions checks for timed out transactions
func (nm *NonceManager) FindTimeoutTransactions(chainID int) []uint64 {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// Find timed out transactions
	now := time.Now()
	var timedOutNonces []uint64

	for nonce, tx := range chainData.pendingTxs {
		if tx.Status == TxPending && now.Sub(tx.CreatedAt) > nm.txTimeout {
			tx.Status = TxTimedOut
			tx.UpdatedAt = now
			log.Printf("Transaction timed out for chain %d, nonce %d: %s", chainID, nonce, tx.Hash.Hex())
			timedOutNonces = append(timedOutNonces, nonce)
		}
	}

	return timedOutNonces
}

// ReuseNonce allows a specific nonce to be reused
func (nm *NonceManager) ReuseNonce(chainID int, nonce uint64) {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// Only reuse if it's the lowest pending nonce
	lowestPending := nm.getLowestPendingNonce(chainData)
	if nonce == lowestPending {
		if chainData.currentNonce > nonce {
			chainData.currentNonce = nonce
			log.Printf("Nonce %d for chain %d set for reuse", nonce, chainID)
		}
	} else {
		log.Printf("Cannot reuse nonce %d for chain %d - not the lowest pending (%d)",
			nonce, chainID, lowestPending)
	}

	// Remove from pending
	delete(chainData.pendingTxs, nonce)
}

// SyncWithBlockchain synchronizes nonce state with the blockchain
func (nm *NonceManager) SyncWithBlockchain(ctx context.Context, chainID int, client *ethclient.Client, address common.Address) error {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	// Fetch the current nonce from the blockchain
	nonce, err := client.PendingNonceAt(ctx, address)
	if err != nil {
		return fmt.Errorf("failed to get pending nonce: %v", err)
	}

	log.Printf("Blockchain nonce for chain %d: %d, our nonce: %d",
		chainID, nonce, chainData.currentNonce)

	// Update our nonce if needed
	if nonce > chainData.currentNonce {
		log.Printf("Updating nonce for chain %d: %d -> %d", chainID, chainData.currentNonce, nonce)
		chainData.currentNonce = nonce
	}

	// Update last sync time
	chainData.lastSync = time.Now()
	return nil
}

// getLowestPendingNonce finds the lowest nonce that is still pending
func (nm *NonceManager) getLowestPendingNonce(chainData *chainNonceData) uint64 {
	var lowestNonce uint64
	foundFirst := false

	for nonce := range chainData.pendingTxs {
		if !foundFirst || nonce < lowestNonce {
			lowestNonce = nonce
			foundFirst = true
		}
	}

	return lowestNonce
}

// GetPendingTransactionsCount returns the number of pending transactions for a chain
func (nm *NonceManager) GetPendingTransactionsCount(chainID int) int {
	// Ensure chain is initialized
	nm.initializeChain(chainID)

	// Get chain data
	nm.mu.RLock()
	chainData := nm.chains[chainID]
	nm.mu.RUnlock()

	// Lock the chain-specific mutex for reading
	chainData.mu.Lock()
	defer chainData.mu.Unlock()

	return len(chainData.pendingTxs)
}
