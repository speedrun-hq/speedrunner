package fulfiller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// TransactionInfo holds information about a tracked transaction
type TransactionInfo struct {
	Address common.Address
	Nonce   uint64
	Time    int64
}

// NonceManager manages nonces for transactions across different chains
type NonceManager struct {
	mu     sync.Mutex
	nonces map[int]map[common.Address]uint64
	txs    map[int]map[common.Hash]*TransactionInfo
}

// NewNonceManager creates a new nonce manager
func NewNonceManager() *NonceManager {
	return &NonceManager{
		nonces: make(map[int]map[common.Address]uint64),
		txs:    make(map[int]map[common.Hash]*TransactionInfo),
	}
}

// GetNonce gets the next nonce for a given chain and address
func (nm *NonceManager) GetNonce(ctx context.Context, chainID int, client *ethclient.Client, address common.Address) (uint64, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Initialize chain maps if they don't exist
	if _, exists := nm.nonces[chainID]; !exists {
		nm.nonces[chainID] = make(map[common.Address]uint64)
	}
	if _, exists := nm.txs[chainID]; !exists {
		nm.txs[chainID] = make(map[common.Hash]*TransactionInfo)
	}

	// Get the current nonce from the blockchain
	nonce, err := client.PendingNonceAt(ctx, address)
	if err != nil {
		return 0, fmt.Errorf("failed to get pending nonce: %w", err)
	}

	// Update our local nonce if it's higher
	if storedNonce, exists := nm.nonces[chainID][address]; !exists || nonce > storedNonce {
		nm.nonces[chainID][address] = nonce
	}

	return nm.nonces[chainID][address], nil
}

// TrackTransaction tracks a transaction with its nonce and address
func (nm *NonceManager) TrackTransaction(chainID int, txHash common.Hash, address common.Address, nonce uint64) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if _, exists := nm.txs[chainID]; !exists {
		nm.txs[chainID] = make(map[common.Hash]*TransactionInfo)
	}
	nm.txs[chainID][txHash] = &TransactionInfo{
		Address: address,
		Nonce:   nonce,
		Time:    time.Now().Unix(),
	}
}

// MarkTransactionConfirmed marks a transaction as confirmed and updates the nonce
func (nm *NonceManager) MarkTransactionConfirmed(chainID int, txHash common.Hash) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	txInfo, exists := nm.txs[chainID][txHash]
	if !exists {
		return fmt.Errorf("transaction %s not found for chain %d", txHash.Hex(), chainID)
	}

	// Update the nonce for the specific address
	if storedNonce, exists := nm.nonces[chainID][txInfo.Address]; exists && storedNonce == txInfo.Nonce {
		nm.nonces[chainID][txInfo.Address] = txInfo.Nonce + 1
	}

	// Clean up the transaction
	delete(nm.txs[chainID], txHash)
	return nil
}

// MarkTransactionFailed marks a transaction as failed and allows the nonce to be reused
func (nm *NonceManager) MarkTransactionFailed(chainID int, txHash common.Hash) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	txInfo, exists := nm.txs[chainID][txHash]
	if !exists {
		return fmt.Errorf("transaction %s not found for chain %d", txHash.Hex(), chainID)
	}

	// Update the nonce for the specific address
	if storedNonce, exists := nm.nonces[chainID][txInfo.Address]; exists && storedNonce == txInfo.Nonce {
		nm.nonces[chainID][txInfo.Address] = txInfo.Nonce - 1
	}

	// Clean up the transaction
	delete(nm.txs[chainID], txHash)
	return nil
}

// GetPendingTransactions returns all pending transactions for a chain
func (nm *NonceManager) GetPendingTransactions(chainID int) map[common.Hash]*TransactionInfo {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if txs, exists := nm.txs[chainID]; exists {
		return txs
	}
	return make(map[common.Hash]*TransactionInfo)
}

// CleanupOldTransactions removes transactions older than the specified duration
func (nm *NonceManager) CleanupOldTransactions(chainID int, maxAge time.Duration) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if txs, exists := nm.txs[chainID]; exists {
		now := time.Now().Unix()
		for txHash, txInfo := range txs {
			if time.Duration(now-txInfo.Time)*time.Second > maxAge {
				delete(txs, txHash)
			}
		}
	}
}
