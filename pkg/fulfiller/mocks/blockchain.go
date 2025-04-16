package mocks

import (
	"math/big"
)

// Address represents an Ethereum address without depending on go-ethereum
type Address struct {
	hex string
}

// NewAddress creates a new address from a hex string
func NewAddress(hex string) Address {
	return Address{hex: hex}
}

// Hex returns the hex string representation of the address
func (a Address) Hex() string {
	return a.hex
}

// Hash represents an Ethereum hash without depending on go-ethereum
type Hash struct {
	hex string
}

// NewHash creates a new hash from a hex string
func NewHash(hex string) Hash {
	return Hash{hex: hex}
}

// Hex returns the hex string representation of the hash
func (h Hash) Hex() string {
	return h.hex
}

// MockClient represents a client for blockchain interactions
type MockClient struct {
	LatestBlockNumber *big.Int
	GasPrice          *big.Int
	Nonce             uint64
	Balance           *big.Int
}

// NewMockClient creates a new mock client
func NewMockClient() *MockClient {
	return &MockClient{
		LatestBlockNumber: big.NewInt(1000),
		GasPrice:          big.NewInt(2000000000), // 2 Gwei
		Nonce:             0,
		Balance:           big.NewInt(1000000000000000000), // 1 ETH
	}
}

// MockTransactor represents transaction options
type MockTransactor struct {
	From     Address
	GasPrice *big.Int
	GasLimit uint64
	Value    *big.Int
	Nonce    uint64
}

// NewMockTransactor creates a new mock transactor
func NewMockTransactor() *MockTransactor {
	return &MockTransactor{
		From:     NewAddress("0x1111111111111111111111111111111111111111"),
		GasPrice: big.NewInt(2000000000), // 2 Gwei
		GasLimit: 300000,
		Value:    big.NewInt(0),
		Nonce:    0,
	}
}

// MockTransaction represents a blockchain transaction
type MockTransaction struct {
	Hash     Hash
	From     Address
	To       Address
	Value    *big.Int
	GasPrice *big.Int
	GasLimit uint64
	Data     []byte
}

// NewMockTransaction creates a new mock transaction
func NewMockTransaction(from, to Address, value *big.Int) *MockTransaction {
	return &MockTransaction{
		Hash:     NewHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		From:     from,
		To:       to,
		Value:    value,
		GasPrice: big.NewInt(2000000000), // 2 Gwei
		GasLimit: 300000,
		Data:     []byte{},
	}
}

// MockReceipt represents a transaction receipt
type MockReceipt struct {
	TxHash    Hash
	Status    uint64
	GasUsed   uint64
	BlockHash Hash
}

// NewMockReceipt creates a new mock receipt
func NewMockReceipt(txHash Hash, success bool) *MockReceipt {
	status := uint64(0)
	if success {
		status = 1
	}
	return &MockReceipt{
		TxHash:    txHash,
		Status:    status,
		GasUsed:   150000,
		BlockHash: NewHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
	}
}

// MockContract represents a blockchain contract
type MockContract struct {
	Address      Address
	CallResults  map[string][]interface{}
	Transactions map[string]*MockTransaction
}

// NewMockContract creates a new mock contract
func NewMockContract(address Address) *MockContract {
	return &MockContract{
		Address:      address,
		CallResults:  make(map[string][]interface{}),
		Transactions: make(map[string]*MockTransaction),
	}
}

// SetCallResult sets the result for a specific method call
func (c *MockContract) SetCallResult(method string, result []interface{}) {
	c.CallResults[method] = result
}

// Call simulates a contract call
func (c *MockContract) Call(_ interface{}, result *[]interface{}, method string, args ...interface{}) error {
	if res, ok := c.CallResults[method]; ok {
		*result = res
		return nil
	}
	// Default allowance is 0
	if method == "allowance" {
		*result = []interface{}{big.NewInt(0)}
	}
	return nil
}

// Transact simulates a contract transaction
func (c *MockContract) Transact(_ interface{}, method string, args ...interface{}) (*MockTransaction, error) {
	tx := NewMockTransaction(
		NewAddress("0x1111111111111111111111111111111111111111"),
		c.Address,
		big.NewInt(0),
	)
	c.Transactions[method] = tx
	return tx, nil
}

// MockChainConfig represents a chain configuration
type MockChainConfig struct {
	ChainID       int
	IntentAddress string
	Client        *MockClient
	Auth          *MockTransactor
	Contract      *MockContract
}

// NewMockChainConfig creates a new mock chain configuration
func NewMockChainConfig(chainID int) *MockChainConfig {
	address := NewAddress("0x2222222222222222222222222222222222222222")
	return &MockChainConfig{
		ChainID:       chainID,
		IntentAddress: "0x3333333333333333333333333333333333333333",
		Client:        NewMockClient(),
		Auth:          NewMockTransactor(),
		Contract:      NewMockContract(address),
	}
}

// UpdateGasPrice simulates updating the gas price
func (c *MockChainConfig) UpdateGasPrice() (*big.Int, error) {
	// Simulate gas price update
	newGasPrice := new(big.Int).Mul(c.Client.GasPrice, big.NewInt(2))
	c.Auth.GasPrice = newGasPrice
	c.Client.GasPrice = newGasPrice
	return newGasPrice, nil
}

// WaitMined simulates waiting for a transaction to be mined
func WaitMined(tx *MockTransaction) (*MockReceipt, error) {
	return NewMockReceipt(tx.Hash, true), nil
}
